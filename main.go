package main

import (
	"os"

	synckr "github.com/koukihai/synckr/synckr"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// main is the pricipal entry point
func main() {
	config, err := synckr.LoadConfiguration("./synckr.conf.json")
	if err != nil {
		log.Fatal("Unable to load configuration")
	}

	if config.LogOutput != "" {
		logfile, err := os.OpenFile("synckr.log", os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Info("Failed to log to file, using default stderr")
		} else {
			log.Out = logfile
		}
	}

	client, err := synckr.GetClient(&config)
	if err != nil {
		log.Fatal("Unable to instanciate flickrClient")
	}

	synckr.Process(&config, &client, log)

}
