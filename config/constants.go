package config

import "path/filepath"

const (
	// StaticVersionNumber is the version of Owncast that is used when it's not overwritten via build-time settings.
	StaticVersionNumber = "0.0.12" // Shown when you build from develop
	// WebRoot is the web server root directory.
	WebRoot = "webroot"
	// HugoRoot is the secondary web server root directory, a fallback if the file was not found in webroot.
	HugoRoot = "hugo/public"
	// HugoDir is the directory with the hugo source files, where the hugo build will be run
	HugoDir = "hugo"
	// HugoTemplateDir is what HugoDir is initialized to if it doesn't exist
	HugoTemplateDir = "hugo-template"
	// FfmpegSuggestedVersion is the version of ffmpeg we suggest.
	FfmpegSuggestedVersion = "v4.1.5" // Requires the v
	// DataDirectory is the directory we save data to.
	DataDirectory = "data"
	// EmojiDir is relative to the webroot.
	EmojiDir = "/img/emoji"
)

var (
	// BackupDirectory is the directory we write backup files to.
	BackupDirectory = filepath.Join(DataDirectory, "backup")

	// HLSStoragePath is the directory HLS video is written to.
	HLSStoragePath = filepath.Join(DataDirectory, "hls")
)
