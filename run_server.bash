set -e

mkdir -p server/_html
gopherjs build -m -o server/_html/frontend.js github.com/runningwild/flow/frontend
cp frontend/index.html server/_html/index.html
cd server
# go run main.go --kubectl /Users/jwills/google-cloud-sdk/bin/kubectl
go run main.go
