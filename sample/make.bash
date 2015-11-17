set -e

go get ./...
rm -rf bin
mkdir -p bin
mkdir -p images
for val in frontend processor/processor-server storage/storage-server
do
	echo $val
	base=`basename $val`

	# Build and copy over binary
	go build -o bin/$base ./$val
	mkdir -p image-$val/rootfs/bin
	mv bin/$base image-$val/rootfs/bin/$base

	# Copy over manifest
	cp ./$val/manifest image-$val/manifest

	actool build --overwrite image-$val images/$base.aci
	rm -f images/$base.aci.asc
	gpg --armor --detach-sign images/$base.aci
	pusher --aci images/$base.aci
	rm -rf image-$val
done

