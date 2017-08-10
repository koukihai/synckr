package synckrlib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sort"

	"io/ioutil"

	"encoding/json"

	"gopkg.in/masci/flickr.v2"
	"gopkg.in/masci/flickr.v2/photos"
	"gopkg.in/masci/flickr.v2/photosets"

	"github.com/sirupsen/logrus"
)

var log = logrus.New()

// Config contains all configuration parameters for
// the application.
// It's filled from the json config file through LoadConfiguration
type Config struct {
	APIKey           string   `json:"api_key"`
	APISecret        string   `json:"api_secret"`
	PhotoLibraryPath string   `json:"photo_library_path"`
	OAuthToken       string   `json:"oauth_token"`
	OAuthTokenSecret string   `json:"oauth_token_secret"`
	SkipDirs         []string `json:"skip_dirs"`
	Extensions       []string `json:"extensions"`
	DeleteDupes      bool     `json:"delete_dupes"`
	LogLevel         string   `json:"log_level"`
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

// LoadConfiguration reads json configuration files and returns
// a SynckrConfig pointer
func LoadConfiguration(filename string) (Config, error) {
	var config Config
	raw, err := ioutil.ReadFile(filename)

	if err != nil {
		log.Error(err.Error())
	} else {
		json.Unmarshal(raw, &config)
		if config.APIKey == "" || config.APISecret == "" {
			log.WithFields(logrus.Fields{
				"api_key":    config.APIKey,
				"api_secret": config.APISecret,
			}).Fatal("Please visit https://www.flickr.com/services/apps/create/noncommercial/ to apply for a non-commercial key.")
		}
	}
	return config, err
}

// GetClient returns a flickr client
func GetClient(config *Config) (flickr.FlickrClient, error) {
	var err error
	client := flickr.NewFlickrClient(config.APIKey, config.APISecret)

	if config.OAuthToken == "" || config.OAuthTokenSecret == "" {
		oauthToken, oauthTokenSecret, err := GetOAuthToken(client)
		if err != nil {
			log.Fatal("Could not generate OAuthToken")
		}

		log.WithFields(logrus.Fields{
			"oauth_token":        oauthToken,
			"oauth_token_secret": oauthTokenSecret,
		}).Info("Please update synckr.conf.json with the corresponding oauth_token and oauth_token_secret")

		config.OAuthToken = oauthToken
		config.OAuthTokenSecret = oauthTokenSecret

	}

	client.OAuthToken = config.OAuthToken
	client.OAuthTokenSecret = config.OAuthTokenSecret
	return *client, err
}

// GetOAuthToken helps you creating an OAuthToken
func GetOAuthToken(client *flickr.FlickrClient) (string, string, error) {
	// get a request token
	tok, err := flickr.GetRequestToken(client)
	if err != nil {
		return "", "", err
	}

	// build the authorization URL
	url, err := flickr.GetAuthorizeUrl(client, tok)
	if err != nil {
		return "", "", err
	}

	// ask user to hit the authorization url with
	// their browser, authorize this application and coming
	// back with the confirmation token
	var oauthVerifier string
	fmt.Println("Open your browser at this url:", url)
	fmt.Print("Then, insert the code:")
	fmt.Scanln(&oauthVerifier)

	// finally, get the access token
	accessTok, err := flickr.GetAccessToken(client, tok, oauthVerifier)
	fmt.Println("Successfully retrieved OAuth token", accessTok.OAuthToken, accessTok.OAuthTokenSecret)

	return accessTok.OAuthToken, accessTok.OAuthTokenSecret, err

}

// RetrieveFromFlickr returns a map associating the title of an album to
// a FlickrPhotoset{id string, photos []string}
func RetrieveFromFlickr(client *flickr.FlickrClient) map[string]FlickrPhotoset {

	result := make(map[string]FlickrPhotoset)

	// Retrieve all photos and albums from flickr
	log.Info("Retrieving photosets from flickr...")
	respSetList, err := photosets.GetList(client, true, "", 0)
	if err != nil {
		log.Fatal("Could not retrieve album list. " + respSetList.ErrorMsg())
	} else {
		for _, ps := range respSetList.Photosets.Items {
			photoset := FlickrPhotoset{ID: ps.Id}
			var photolist []FlickrPhoto

			currentPage := 1
			respPhotoList, err := photosets.GetPhotos(client, true, ps.Id, "", currentPage)
			for len(respPhotoList.Photoset.Photos) > 0 {
				if err != nil {
					log.Fatal("Could not retrieve the photo list. " + respPhotoList.ErrorMsg())
				} else {
					for _, ph := range respPhotoList.Photoset.Photos {
						photolist = append(photolist, FlickrPhoto{ph.Id, ph.Title})
					}
					currentPage++
					respPhotoList, err = photosets.GetPhotos(client, true, ps.Id, "", currentPage)
				}
			}

			sort.Sort(FlickrPhotosByTitle(photolist))
			photoset = FlickrPhotoset{ID: ps.Id, Photos: photolist}
			result[ps.Title] = photoset
			log.Info("[OK] Loaded ", len(photoset.Photos), " photos from ", ps.Title)
		}
	}
	log.Info("[OK] Loaded ", len(result), " photosets.")
	return result
}

// DeleteDupes deletes duplicate files from an album
func DeleteDupes(client *flickr.FlickrClient, fromFlickr *map[string]FlickrPhotoset) {

	for albumName, flickrAlbum := range *fromFlickr {
		for phi, ph := range flickrAlbum.Photos {
			if phi > 0 && ph.Title == flickrAlbum.Photos[phi-1].Title {
				log.WithFields(logrus.Fields{
					"album.name": albumName,
					"photo.name": ph.Title,
				}).Warn("[DELETE] Deleting duplicate.")
				photos.Delete(client, ph.ID)
			}
		}
	}
}

// CreateAlbum will create an album and set the photo as the primary photo
func CreateAlbum(client *flickr.FlickrClient, albumName string, photoID string) (string, error) {
	result := ""
	respS, err := photosets.Create(client, albumName, "", photoID)
	if err != nil {
		log.WithFields(logrus.Fields{
			"code":    respS.ErrorCode(),
			"message": respS.ErrorMsg(),
		}).Error("[ERROR] Failed creating set.")
	} else {
		log.WithFields(logrus.Fields{
			"album.name": albumName,
			"album.id":   respS.Set.Id,
		}).Info("[OK] Set created")
		result = respS.Set.Id
	}
	return result, err
}

// AppendPhotoIntoExistingAlbum will add a photo into an existing album
func AppendPhotoIntoExistingAlbum(client *flickr.FlickrClient, albumID string, photoID string) (string, error) {
	respAdd, err := photosets.AddPhoto(client, albumID, photoID)
	if err != nil {
		log.WithFields(logrus.Fields{
			"code":    respAdd.ErrorCode(),
			"message": respAdd.ErrorMsg(),
		}).Error("[ERROR] Failed adding photo to the set.")
	} else {
		log.WithFields(logrus.Fields{
			"photo.id": photoID,
			"set.id":   albumID,
		}).Info("[OK] Added photo to existing set.")
	}
	return albumID, err
}

// UploadPhoto uploads a given path into a given album. It creates a new album if none is provided
func UploadPhoto(client *flickr.FlickrClient, albumID string, path string) (string, string, error) {
	photoID := ""
	currentDir := filepath.Base(filepath.Dir(path))

	resp, err := flickr.UploadFile(client, path, nil)
	if err != nil {
		log.WithFields(logrus.Fields{
			"path":     path,
			"album.id": albumID,
			"error":    err,
		}).Error("[ERROR] Photo upload failed.")
		if resp != nil {
			log.WithFields(logrus.Fields{
				"code":    resp.ErrorCode(),
				"message": resp.ErrorMsg(),
			}).Error("[ERROR] Response contents")
		} else {
			log.Error("[ERROR] Empty response")
		}
	} else {
		log.WithFields(logrus.Fields{
			"path":     path,
			"album.id": albumID,
			"photo.id": resp.ID,
		}).Info("[OK] Photo uploaded")
		photoID = resp.ID

		// AlbumID is not provided, we create a new album
		if albumID == "" {
			albumID, err = CreateAlbum(client, currentDir, resp.ID)
		} else {
			// AlbumID is provided, we append the photo to the albumID
			albumID, err = AppendPhotoIntoExistingAlbum(client, albumID, resp.ID)
		}
	}

	return albumID, photoID, err
}

// SetLogLevel will update the log level according to the json
// configuration file
func SetLogLevel(config *Config, log *logrus.Logger) {
	level, err := logrus.ParseLevel(config.LogLevel)
	if err != nil {
		log.Level = logrus.InfoLevel
	} else {
		log.Level = level
	}
}

// Process will scan the files within the local drive and identify if they need to be uploaded
// to flickr.
// If a file already exists in flickr
//   --> it will be skipped
// If a file doesn't exist yet
//   --> it will be uploaded into an album which title will be the parent directory name
func Process(config *Config, client *flickr.FlickrClient, parentlog *logrus.Logger) (map[string]FlickrPhotoset, error) {
	var err error

	if parentlog != nil {
		log = parentlog
	}

	SetLogLevel(config, log)

	fromFlickr := RetrieveFromFlickr(client)
	if config.PhotoLibraryPath == "" {
		log.WithFields(logrus.Fields{
			"photo_library_path": config.PhotoLibraryPath,
		}).Fatal("Please update synckr.conf.json")
	}

	if config.DeleteDupes {
		DeleteDupes(client, &fromFlickr)
	}

	// Walk photolibrarypath using a lambda as walk function
	_, err = os.Stat(config.PhotoLibraryPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.WithField("path", config.PhotoLibraryPath).Fatal("Path does not exist")
		} else {
			log.WithField("path", config.PhotoLibraryPath).Fatal("Cannot access path. ", err.Error())
		}
	}
	skipDirs := []string{"@eaDir"}
	if len(config.SkipDirs) != 0 {
		skipDirs = config.SkipDirs
	}

	allowedExtensions := []string{".png", ".jpg", ".jpeg"}
	if len(config.Extensions) != 0 {
		allowedExtensions = config.Extensions
	}

	filepath.Walk(config.PhotoLibraryPath, func(path string, info os.FileInfo, err error) error {

		if info.IsDir() {
			dir := filepath.Base(path)
			for _, d := range skipDirs {
				if d == dir {
					return filepath.SkipDir
				}
			}
		}

		// Only treat files
		if !info.IsDir() {
			isAllowedExt := false
			isRootDir := false

			if filepath.Dir(path) == config.PhotoLibraryPath {
				log.WithField("path", path).Info("[SKIP] Root folder not processed.")
				isRootDir = true
			}

			for _, i := range allowedExtensions {
				if strings.ToLower(filepath.Ext(path)) == i {
					isAllowedExt = true
				}
			}

			if !isRootDir && !isAllowedExt {
				log.WithField("path", path).Info("[SKIP] File not supported.")
			}

			// Files on the base root path will not be uploaded
			if isAllowedExt && !isRootDir {
				photoName := strings.Split(filepath.Base(path), ".")[0]
				currentDir := filepath.Base(filepath.Dir(path))

				uploadNeeded := false
				destinationAlbum := ""

				// Check if file need to be uploaded.
				_, albumPresent := fromFlickr[currentDir]

				// The album is present in flickr. has the photo already been uploaded?
				if albumPresent {
					phi := sort.Search(len(fromFlickr[currentDir].Photos), func(i int) bool {
						return fromFlickr[currentDir].Photos[i].Title >= photoName
					})
					if phi == len(fromFlickr[currentDir].Photos) {
						uploadNeeded = true
						destinationAlbum = fromFlickr[currentDir].ID
					} else {
						log.WithFields(logrus.Fields{
							"photo.name": photoName,
							"album.name": currentDir,
						}).Info("[SKIP]Already uploded")
					}
				} else {
					// The album is not present in flickr. The photo needs to be uploaded
					uploadNeeded = true
					destinationAlbum = ""
				}

				if uploadNeeded {
					attemptNb := 0
					albumID, photoID, err := UploadPhoto(client, destinationAlbum, path)
					// Try 5 times before skipping
					// TODO: configure retries
					for err != nil && attemptNb < 5 {
						log.WithFields(logrus.Fields{
							"attempt": attemptNb,
						}).Warn("[WARNING] Upload attempt failed")
						// Sleep 5 minutes after a connection error
						// TODO: configure retry_interval
						time.Sleep(5 * time.Minute)
						attemptNb++
						albumID, photoID, err = UploadPhoto(client, destinationAlbum, path)
					}
					if err != nil {
						log.WithFields(logrus.Fields{
							"attempt":    attemptNb,
							"photo.name": photoName,
							"album.name": currentDir,
						}).Error("[ERROR] Upload failed")
					} else {
						photolist := fromFlickr[currentDir].Photos
						photolist = append(photolist, FlickrPhoto{photoID, photoName})
						fromFlickr[currentDir] = FlickrPhotoset{albumID, photolist}
					}
				}

			}

		}
		return err
	})

	return fromFlickr, err
}
