package synckr_test

import (
	"testing"

	synckr "github.com/koukihai/synckr/synckr"
	"github.com/sirupsen/logrus"
)

func TestLoadConfiguration(t *testing.T) {
	_, err := synckr.LoadConfiguration("this_file_doesnot_exit")
	if err == nil {
		t.Error("File does not exist. Should have raised an error")
	}

	_, err = synckr.LoadConfiguration("test/synckr_test.conf.json")
	if err != nil {
		t.Error("File exists. Should not raise an error")
	}
}

func TestRetrieveFromFlickr(t *testing.T) {
	config, err := synckr.LoadConfiguration("../synckr/test/synckr_test.conf.json")
	if err != nil {
		t.Error("Unable to load configuration")
	}

	client, err := synckr.GetClient(&config)
	if err != nil {
		t.Error("Unable to instanciate flickrClient")
	}

	fromFlickr := synckr.RetrieveFromFlickr(&client, &config)
	if len(fromFlickr["Mugen"].Photos) != 4 {
		t.Error("Test album contains should contain exactly 4 photos")
	}
}

func TestSetLogLevel(t *testing.T) {
	var config synckr.Config
	log := logrus.New()

	config.LogLevel = "error"
	synckr.SetLogLevel(&config, log)
	if log.Level != logrus.ErrorLevel {
		t.Error("ERROR level not parsed correctly. ", config.LogLevel, log.Level)
	}

	config.LogLevel = ""
	synckr.SetLogLevel(&config, log)
	if log.Level != logrus.InfoLevel {
		t.Error("Default level should be INFO")
	}

}
