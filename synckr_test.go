package synckr_test

import (
	"testing"

	"github.com/koukihai/synckr"
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
	config, err := synckr.LoadConfiguration("./synckr.conf.json")
	if err != nil {
		t.Error("Unable to load configuration")
	}

	client, err := synckr.GetClient(&config)
	if err != nil {
		t.Error("Unable to instanciate flickrClient")
	}

	fromFlickr := synckr.RetrieveFromFlickr(&client)
	if len(fromFlickr["Song Charts #1 - Mar. 17, 2008"].Photos) != 10 {
		t.Error("Test album contains more than 10 photos")
	}
}
