#!/bin/sh

go get -u google.golang.org/protobuf/cmd/protoc-gen-go

protoc --go_out=../pb/mcs mcs.proto
protoc --go_out=../pb/checkin checkin.proto
protoc --go_out=../pb/checkin android_checkin.proto

