package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sort"

	"io/ioutil"

	"encoding/json"

	"gopkg.in/masci/flickr.v2"
	"gopkg.in/masci/flickr.v2/photos"
	"gopkg.in/masci/flickr.v2/photosets"
)

var config *SynckrConfig
var client *flickr.FlickrClient

// SynckrConfig contains all configuration parameters for
// the application. It picks data from both json files
type SynckrConfig struct {
	APIKey           string `json:"api_key"`
	APISecret        string `json:"api_secret"`
	PhotoLibraryPath string `json:"photo_library_path"`
	OAuthToken       string `json:"oauth_token"`
	OAuthTokenSecret string `json:"oauth_token_secret"`
}

// FlickrPhotoset contains the ID and the list of photo titles
// for a given photoset retrieved from flickr
type FlickrPhotoset struct {
	ID     string
	Photos []FlickrPhoto
}

// FlickrPhoto contains the ID and the title for a given
// photo retrieved from flickr
type FlickrPhoto struct {
	ID    string
	Title string
}

// FlickrPhotosByTitle implements Sort interface to sort photos
// by their title
type FlickrPhotosByTitle []FlickrPhoto

func (a FlickrPhotosByTitle) Len() int           { return len(a) }
func (a FlickrPhotosByTitle) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a FlickrPhotosByTitle) Less(i, j int) bool { return a[i].Title < a[j].Title }

// Leave exits the program with an error message
func Leave(message string) {
	fmt.Println(message)
	os.Exit(1)
}

// LoadConfiguration reads json configuration files and returns
// a SynckrConfig pointer
func LoadConfiguration() {
	// If config has not been loaded yet, retrieve it
	if config == nil {
		raw, err := ioutil.ReadFile("./synckr.conf.json")

		if err != nil {
			Leave(err.Error())
		}

		json.Unmarshal(raw, &config)
		if config.APIKey == "" || config.APISecret == "" || config.OAuthToken == "" || config.OAuthTokenSecret == "" {
			Leave("Please set FLICKRGO_API_KEY, FLICKRGO_API_SECRET, " +
				"FLICKRGO_OAUTH_TOKEN and FLICKRGO_OAUTH_TOKEN_SECRET env vars")
		}
	}
}

// GetClient returns a flickr client
func GetClient() {
	// If client has not been set yet, build it
	if client == nil {
		if config == nil {
			LoadConfiguration()
		}
		client = flickr.NewFlickrClient(config.APIKey, config.APISecret)
		client.OAuthToken = config.OAuthToken
		client.OAuthTokenSecret = config.OAuthTokenSecret
	}
}

// RetrieveFromFlickr returns a map associating the title of an album to
// a FlickrPhotoset{id string, photos []string}
func RetrieveFromFlickr() map[string]FlickrPhotoset {
	if client == nil {
		GetClient()
	}

	result := make(map[string]FlickrPhotoset)

	// Retrieve all photos and albums from flickr
	fmt.Println("Retrieving photosets from flickr...")
	respSetList, err := photosets.GetList(client, true, "", 0)
	if err != nil {
		Leave("Could not retrieve the album list. " + respSetList.ErrorMsg())
	} else {
		for _, ps := range respSetList.Photosets.Items {
			photoset := FlickrPhotoset{ID: ps.Id}

			respPhotoList, err := photosets.GetPhotos(client, true, ps.Id, "", 0)
			if err != nil {
				Leave("Could not retrieve the photo list. " + respPhotoList.ErrorMsg())
			} else {
				var photolist []FlickrPhoto
				for _, ph := range respPhotoList.Photoset.Photos {
					photolist = append(photolist, FlickrPhoto{ph.Id, ph.Title})
				}
				sort.Sort(FlickrPhotosByTitle(photolist))
				photoset = FlickrPhotoset{ID: ps.Id, Photos: photolist}
			}

			result[ps.Title] = photoset
		}
	}
	fmt.Println("[OK] Loaded ", len(result), "photosets.")
	return result
}

// DeleteDupes deletes duplicate files from an album
func DeleteDupes() {
	if client == nil {
		GetClient()
	}
	fromFlickr := RetrieveFromFlickr()
	for albumName, flickrAlbum := range fromFlickr {
		fmt.Println("In album: ", albumName, ": ", flickrAlbum.Photos)
		for phi, ph := range flickrAlbum.Photos {
			if phi > 0 && ph.Title == flickrAlbum.Photos[phi-1].Title {
				fmt.Println("Duplicate detected in ", albumName, ". Deleting  ", ph.Title)
				photos.Delete(client, ph.ID)
			}
		}
	}
}

// UploadPhoto uploads a given path into a given album
// it creates a new album if none is provided
func UploadPhoto(albumID string, path string) (string, string, error) {
	result := albumID
	photoID := ""
	currentDir := filepath.Base(filepath.Dir(path))

	if client == nil {
		GetClient()
	}

	resp, err := flickr.UploadFile(client, path, nil)
	if err != nil {
		fmt.Println("[ERROR]Failed uploading:", err)
		if resp != nil {
			fmt.Println(resp.ErrorMsg())
		}
	} else {
		fmt.Println("[OK] Photo uploaded with ID#", resp.ID)
		photoID = resp.ID

		// AlbumID is not provided, we create a new album
		if albumID == "" {
			respS, err := photosets.Create(client, currentDir, "", resp.ID)
			if err != nil {
				fmt.Println("[ERROR] Failed creating set:", respS.ErrorMsg())
			} else {
				fmt.Println("[OK] Set created. ID#", respS.Set.Id)
				result = respS.Set.Id
			}
		} else {
			// AlbumID is provided, we append the photo to the albumID
			respAdd, err := photosets.AddPhoto(client, albumID, resp.ID)
			if err != nil {
				Leave("Failed adding photo to the set:" + respAdd.ErrorMsg())
			} else {
				fmt.Println("[OK] Added photo ID#", resp.ID, "to existing set ID#", albumID)
			}
		}
	}

	return result, photoID, err
}

// main is the pricipal entry point
func main() {

	fromFlickr := RetrieveFromFlickr()

	// Walk photolibrarypath using a lambda as walk function
	filepath.Walk(config.PhotoLibraryPath, func(path string, info os.FileInfo, err error) error {
		err = nil
		// Only treat files
		if !info.IsDir() {
			photoName := strings.Split(filepath.Base(path), ".")[0]

			// Files on the base root path will not be uploaded
			if filepath.Dir(path) != config.PhotoLibraryPath {
				currentDir := filepath.Base(filepath.Dir(path))
				// Check if file need to be uploaded.
				_, albumPresent := fromFlickr[currentDir]

				// The album is present in flickr. has the photo already been uploaded?
				if albumPresent {
					phi := sort.Search(len(fromFlickr[currentDir].Photos), func(i int) bool {
						return fromFlickr[currentDir].Photos[i].Title >= photoName
					})
					if phi == len(fromFlickr[currentDir].Photos) {
						UploadPhoto(fromFlickr[currentDir].ID, path)
					} else {
						fmt.Println("[SKIP]Already uploded ", photoName, " in album ", currentDir)
					}
				} else {
					// The album is not present in flickr. The photo needs to be uploaded
					photosetID, photoID, err := UploadPhoto("", path)
					if err != nil {
						fmt.Println("[ERROR] Failed creating new album. ", err)
					} else {
						photolist := fromFlickr[currentDir].Photos
						photolist = append(photolist, FlickrPhoto{photoID, photoName})
						fromFlickr[currentDir] = FlickrPhotoset{photosetID, photolist}
					}
				}

			}

		}
		return nil
	})

}
