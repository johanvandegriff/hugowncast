package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"

	cp "github.com/otiai10/copy"
	log "github.com/sirupsen/logrus"

	"github.com/gohugoio/hugo/commands"
	"github.com/owncast/owncast/auth"
	"github.com/owncast/owncast/config"
	"github.com/owncast/owncast/core/chat"
	"github.com/owncast/owncast/core/data"
	"github.com/owncast/owncast/core/rtmp"
	"github.com/owncast/owncast/core/transcoder"
	"github.com/owncast/owncast/core/user"
	"github.com/owncast/owncast/core/webhooks"
	"github.com/owncast/owncast/models"
	"github.com/owncast/owncast/notifications"
	"github.com/owncast/owncast/utils"
	"github.com/owncast/owncast/yp"

	"github.com/radovskyb/watcher"
)

var (
	_stats       *models.Stats
	_storage     models.StorageProvider
	_transcoder  *transcoder.Transcoder
	_yp          *yp.YP
	_broadcaster *models.Broadcaster
	handler      transcoder.HLSHandler
	fileWriter   = transcoder.FileWriterReceiverService{}
)

// Start starts up the core processing.
func Start() error {
	hugoBuildWatch()

	resetDirectories()

	data.PopulateDefaults()

	if err := data.VerifySettings(); err != nil {
		log.Error(err)
		return err
	}

	if err := setupStats(); err != nil {
		log.Error("failed to setup the stats")
		return err
	}

	// The HLS handler takes the written HLS playlists and segments
	// and makes storage decisions.  It's rather simple right now
	// but will play more useful when recordings come into play.
	handler = transcoder.HLSHandler{}

	if err := setupStorage(); err != nil {
		log.Errorln("storage error", err)
	}

	user.SetupUsers()
	auth.Setup(data.GetDatastore())

	fileWriter.SetupFileWriterReceiverService(&handler)

	if err := createInitialOfflineState(); err != nil {
		log.Error("failed to create the initial offline state")
		return err
	}

	_yp = yp.NewYP(GetStatus)

	if err := chat.Start(GetStatus); err != nil {
		log.Errorln(err)
	}

	// start the rtmp server
	go rtmp.Start(setStreamAsConnected, setBroadcaster)

	rtmpPort := data.GetRTMPPortNumber()
	log.Infof("RTMP is accepting inbound streams on port %d.", rtmpPort)

	webhooks.InitWorkerPool()

	notifications.Setup(data.GetStore())

	return nil
}

func hugoBuild() {
	//run the actual hugo command, similar to "hugo ---source hugo" on the command line
	commands.Execute([]string{"--source", config.HugoDir})
}

func hugoBuildWatch() {
	//if the hugo dir doesn't exist or is empty, copy the template
	listing, err := ioutil.ReadDir(config.HugoDir)
	if err != nil {
		log.Infof("HugoDir (%s) did not exist, copying HugoTemplateDir (%s): %w", config.HugoDir, config.HugoTemplateDir, err)
		err := cp.Copy(config.HugoTemplateDir, config.HugoDir)
		if err != nil {
			log.Fatalf("unable to copy HugoDir (%s) to HugoTemplateDir (%s): %w", config.HugoDir, config.HugoTemplateDir, err)
		}
		go hugoBuild() //run the build after copying the template
	} else if len(listing) == 0 { //this is needed for docker, since it doesn't have permission to overwrite the dir
		log.Infof("HugoDir (%s) was empty, copying HugoTemplateDir (%s)", config.HugoDir, config.HugoTemplateDir)
		listing, _ := ioutil.ReadDir(config.HugoTemplateDir)
		for _, item := range listing {
			log.Infof("  \\-- copying %s", item.Name())
			err := cp.Copy(filepath.Join(config.HugoTemplateDir, item.Name()), filepath.Join(config.HugoDir, item.Name()))
			if err != nil {
				log.Fatalf("unable to copy HugoDir (%s) to HugoTemplateDir (%s): %w", config.HugoDir, config.HugoTemplateDir, err)
			}
		}
		go hugoBuild() //run the build after copying the template
	}

	//set up a file watch (the one built in to hugo would stop watching after an error)
	w := watcher.New()
	w.SetMaxEvents(1) // allow at most 1 event per watch cycle, to avoid building many times

	//exclude the "hugo/public" folder to avoid the build output triggering more builds
	excludeDir, err := filepath.Abs(config.HugoRoot)
	if err != nil {
		log.Fatalf("error getting absolute path of HugoRoot (%s): %w", config.HugoRoot, err)
	}
	exclude_regex := regexp.MustCompile(fmt.Sprintf("^%s.*$", excludeDir))

	//run the hugo build when a file watch event comes in
	go func() {
		for {
			select {
			case event := <-w.Event:
				if !exclude_regex.MatchString(event.Path) {
					log.Infoln(event) // Print the event's info.
					hugoBuild()
				}
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	// Watch test_folder recursively for changes.
	if err := w.AddRecursive(config.HugoDir); err != nil {
		log.Fatalln(err)
	}

	go func() {
		// Start the watching process
		if err := w.Start(config.HugoRefreshTime); err != nil {
			log.Fatalln(err)
		}
	}()
}

func createInitialOfflineState() error {
	transitionToOfflineVideoStreamContent()

	return nil
}

// transitionToOfflineVideoStreamContent will overwrite the current stream with the
// offline video stream state only.  No live stream HLS segments will continue to be
// referenced.
func transitionToOfflineVideoStreamContent() {
	log.Traceln("Firing transcoder with offline stream state")

	_transcoder := transcoder.NewTranscoder()
	_transcoder.SetIdentifier("offline")
	_transcoder.SetLatencyLevel(models.GetLatencyLevel(4))
	_transcoder.SetIsEvent(true)

	offlineFilePath, err := saveOfflineClipToDisk("offline.ts")
	if err != nil {
		log.Fatalln("unable to save offline clip:", err)
	}

	_transcoder.SetInput(offlineFilePath)
	go _transcoder.Start()

	// Copy the logo to be the thumbnail
	logo := data.GetLogoPath()
	if err = utils.Copy(filepath.Join("data", logo), "webroot/thumbnail.jpg"); err != nil {
		log.Warnln(err)
	}

	// Delete the preview Gif
	_ = os.Remove(path.Join(config.WebRoot, "preview.gif"))
}

func resetDirectories() {
	log.Trace("Resetting file directories to a clean slate.")

	// Wipe hls data directory
	utils.CleanupDirectory(config.HLSStoragePath)

	// Remove the previous thumbnail
	logo := data.GetLogoPath()
	if utils.DoesFileExists(logo) {
		err := utils.Copy(path.Join("data", logo), filepath.Join(config.WebRoot, "thumbnail.jpg"))
		if err != nil {
			log.Warnln(err)
		}
	}
}
