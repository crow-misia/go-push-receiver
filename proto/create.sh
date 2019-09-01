#!/bin/sh

if [ ! -f bin/protoc ]; then
  curl -L -o protoc.zip https://github.com/protocolbuffers/protobuf/releases/download/v3.9.1/protoc-3.9.1-linux-x86_64.zip
  unzip protoc.zip
fi

bin/protoc --go_out=plugins=grpc:../pb/checkin android_checkin.proto
bin/protoc --go_out=plugins=grpc:../pb/checkin checkin.proto
bin/protoc --go_out=plugins=grpc:../pb/mcs mcs.proto

