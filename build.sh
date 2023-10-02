#!/bin/bash

architectures=("amd64" "arm64")
goos=("linux" "darwin" "windows")

mkdir ./bin/
for arch in ${architectures[@]}; do
	for os in ${goos[@]}; do
		echo "building for $os/$arch"
		env GOOS=$os GOARCH=$arch go build .
		if [[ $os = "windows" ]]; then
			zip -m ./bin/listme_${os}_${arch}.zip ./listme.exe
		else
			zip -m ./bin/listme_${os}_${arch}.zip ./listme
		fi
	done
done
