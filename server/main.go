package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/appc/spec/schema"
)

var (
	kubectlBin = flag.String("kubectl", "/home/jwills/kubernetes/client/bin/kubectl", "Path to kubectl binary.")
)

func main() {
	flag.Parse()
	if *kubectlBin == "" {
		log.Fatalf("Must specify kubectl binary with --kubectl.")
	}
	log.Printf("Running kubectl from %s", *kubectlBin)
	s := &server{
		kubectl: *kubectlBin,
		files:   http.FileServer(http.Dir(".")),
	}
	log.Printf("serving")
	log.Fatal(http.ListenAndServe(":9090", s))
}

type server struct {
	kubectl string
	files   http.Handler
	kubeMu  sync.Mutex
}

const containerPrefix = "/container/"
const uiPrefix = "/_html/"
const kubectlPrefix = "/kubectl/"

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("Get request: %v", r.URL.String())
	w.Header().Add("Access-Control-Allow-Origin", "*")
	switch {
	case strings.HasPrefix(r.URL.String(), containerPrefix):
		if err := s.handleContainer(w, r); err != nil {
			log.Printf("Failed for: %v", err)
		}

	case strings.HasPrefix(r.URL.String(), uiPrefix):
		s.files.ServeHTTP(w, r)

	case strings.HasPrefix(r.URL.String(), kubectlPrefix):
		s.handleKubectl(w, r)

	default:
		http.NotFound(w, r)
		return
	}
}

var containerRe = regexp.MustCompile(`/container/([^/]+)/([^:]+)(:(.*))?`)

func (s *server) handleContainer(w http.ResponseWriter, r *http.Request) error {
	matches := containerRe.FindStringSubmatch(r.URL.Path)
	if len(matches) != 5 {
		return fmt.Errorf("didn't match container regex")
	}
	domain := matches[1]
	name := domain + "/" + matches[2]
	version := matches[4]
	if version == "" {
		version = "latest"
	}
	fmt.Printf("%s %s %s\n", domain, name, version)
	resp, err := http.Get(fmt.Sprintf("https://%s/meta/meta.html", domain))
	if err != nil {
		return err
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Printf("meta: %s", data)
	var x xHTML
	if err := xml.Unmarshal([]byte(data), &x); err != nil {
		log.Printf("err unmarshaling meta data: %v", err)
	}
	log.Printf("meta: %v", x)
	var target string
	for _, meta := range x.Metas {
		if meta.Name != "ac-discovery" {
			continue
		}
		fields := strings.Fields(meta.Content)
		if len(fields) != 2 {
			continue
		}
		if !strings.HasPrefix(name, fields[0]) {
			continue
		}
		tmpl := fields[1]
		tmpl = strings.Replace(tmpl, "{name}", name, -1)
		tmpl = strings.Replace(tmpl, "{os}", "linux", -1)
		tmpl = strings.Replace(tmpl, "{arch}", "amd64", -1)
		tmpl = strings.Replace(tmpl, "{version}", version, -1)
		tmpl = strings.Replace(tmpl, "{ext}", "aci", -1)
		target = tmpl
		break
	}
	if target == "" {
		return fmt.Errorf("didn't find the appropriate discovery meta")
	}
	log.Printf("%s", target)
	resp, err = http.Get(target)
	if err != nil {
		return fmt.Errorf("unable to find container: %v")
	}
	data, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("unable to read response from server: %v")
	}
	buf := bytes.NewBuffer(data)
	if gzr, err := gzip.NewReader(buf); err == nil {
		if unzipped, err := ioutil.ReadAll(gzr); err == nil {
			data = unzipped
		}
	}
	tr := tar.NewReader(bytes.NewBuffer(data))
	var manifest []byte
	for header, err := tr.Next(); err == nil; header, err = tr.Next() {
		if header == nil {
			break
		}
		if header.Name != "manifest" {
			continue
		}
		manifest, _ = ioutil.ReadAll(tr)
	}
	if manifest == nil {
		return fmt.Errorf("unable to read manifest")
	}
	var im schema.ImageManifest
	if err := json.Unmarshal(manifest, &im); err != nil {
		return fmt.Errorf("unable to parse manifest")
	}
	imIndent, _ := json.MarshalIndent(im, "", "  ")
	log.Printf("%s\n", imIndent)
	if im.App == nil {
		return fmt.Errorf("no app section defined")
	}
	for _, mp := range im.App.MountPoints {
		log.Printf("Mount point: %v", mp.Name)
	}
	for _, port := range im.App.Ports {
		log.Printf("Port: %v@%d", port.Name, port.Port)
	}
	io.Copy(w, bytes.NewBuffer(manifest))
	return nil
}

func (s *server) handleKubectl(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10000000); err != nil {
		fmt.Fprintf(w, "FAIL: Failed to parse multipart form: %v", err)
		return
	}

	// Read all files
	fileData := make(map[string][]byte)
	for _, headers := range r.MultipartForm.File {
		for _, header := range headers {
			reader, err := header.Open()
			if err != nil {
				fmt.Fprintf(w, "FAIL: failed to open file %v: %v", header.Filename, err)
				return
			}
			data, err := ioutil.ReadAll(reader)
			if err != nil {
				fmt.Fprintf(w, "FAIL: failed to read file %v: %v", header.Filename, err)
				return
			}
			if _, ok := fileData[header.Filename]; ok {
				fmt.Fprintf(w, "FAIL: more than one file per filename (%s) is not supported", header.Filename)
				return
			}
			fileData[header.Filename] = data
		}
	}

	s.kubeMu.Lock()
	defer s.kubeMu.Unlock()

	id := "kubeflow"
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("%s", id))
	if err := os.MkdirAll(dir, 0777); err != nil {
		fmt.Fprintf(w, "FAIL: failed to create temporary directory, %s, for staging files: %v", dir, err)
		return
	}
	defer os.RemoveAll(dir)

	pwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(w, "FAIL: unable to get pwd: %v", err)
		return
	}
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintf(w, "FAIL: unable to chdir to %s: %v", dir, err)
		return
	}
	defer os.Chdir(pwd)

	log.Printf("Using temp dir: %v", dir)

	// Write all the files
	for name, data := range fileData {
		log.Printf("File: %q", name)
		log.Printf("%s", data)
		if err := ioutil.WriteFile(name, data, 0777); err != nil {
			fmt.Fprintf(w, "FAIL: failed to write temporary file %s: %v", name, err)
			return
		}
	}

	cmdVals := strings.Fields(r.FormValue("cmd"))
	cmd := exec.Command(s.kubectl, cmdVals...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(w, "FAIL:")
	}
	io.Copy(w, bytes.NewBuffer(output))
	io.Copy(os.Stdout, bytes.NewBuffer(output))
}

type xHTML struct {
	XMLName xml.Name `xml:"html"`
	Metas   []xMeta  `xml:"head>meta"`
}
type xMeta struct {
	Name    string `xml:"name,attr"`
	Content string `xml:"content,attr"`
}
