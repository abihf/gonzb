package main

import (
	"context"
	"os"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/abihf/gonzb/internal/downloader"
	"github.com/abihf/gonzb/internal/nzb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var configFile = kingpin.Flag("config", "config file").Default("config.yml").String()

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	debug := kingpin.Flag("debug", "enable debug mode").Default("false").Bool()

	download := kingpin.Command("download", "download from nzb file")
	fileName := download.Arg("filename", "nzb file").Required().String()

	cmd := kingpin.Parse()

	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	var err error
	switch cmd {
	case "download":
		err = downloadSingle(*fileName)
	}
	if err != nil {
		log.Fatal().Stack().Err(err).Send()
		os.Exit(1)
	}
}

func loadConfig() *downloader.Config {
	var conf downloader.Config
	cFile, err := os.Open(*configFile)
	if err != nil {
		log.Error().Err(err).Str("file", *configFile).Msg("can not open config file")
		os.Exit(2)
	}
	err = yaml.NewDecoder(cFile).Decode(&conf)
	if err != nil {
		log.Error().Err(err).Str("file", *configFile).Msg("can not parse config file")
		os.Exit(3)
	}
	return &conf
}

func downloadSingle(name string) error {
	d := downloader.New(loadConfig())
	defer d.Close()

	n, err := nzb.Open(name)
	if err != nil {
		return err
	}
	return d.Download(context.Background(), n)
}
