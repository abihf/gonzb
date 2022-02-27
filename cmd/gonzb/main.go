package main

import (
	"context"
	"os"

	"github.com/abihf/gonzb/internal/downloader"
	"github.com/abihf/gonzb/internal/nzb"
	"gopkg.in/yaml.v3"
)

func main() {
	file, err := os.Open("test.nzb")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	n, _ := nzb.Parse(file)
	var conf downloader.Config
	cFile, _ := os.Open("config.yml")
	yaml.NewDecoder(cFile).Decode(&conf)
	d := downloader.New(&conf)
	err = d.Download(context.TODO(), n)
	if err != nil {
		panic(err)
	}
}
