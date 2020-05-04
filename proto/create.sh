#!/bin/sh

if [ ! -f bin/protoc ]; then
  curl -L -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v3.11.4/protoc-3.11.4-linux-x86_64.zip
  unzip protoc.zip
  rm protoc.zip
fi

go get -u github.com/golang/protobuf/protoc-gen-go

bin/protoc --go_out=plugins=grpc:../pb/checkin android_checkin.proto
bin/protoc --go_out=plugins=grpc:../pb/checkin checkin.proto
bin/protoc --go_out=plugins=grpc:../pb/mcs mcs.proto

