//go:build darwin || linux

package bc

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Download the video from the response content by spwaning a `ffmpeg` child process.
func saveVideo(respMap map[string]interface{}, path string, logFile io.Writer) error {
	ss := respMap["sources"].([]interface{})
	s := ss[0].(map[string]interface{})
	url := s["src"].(string)
	vid, name := respMap["id"].(string), respMap["name"].(string)
	name = strings.ReplaceAll(name, "/", "-")
	name = vid + "_" + name + ".mp4"
	filePath := filepath.Join(path, name)

	ffmpegCmd := exec.Command("ffmpeg", "-i", url, "-c", "copy", filePath, "-loglevel", "error")
	ffmpegCmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	ffmpegCmd.Stderr = logFile
	if err := ffmpegCmd.Run(); err != nil {
		return err
	}

	return nil
}

// Save the response as `$vid_$name.json` format.
func saveResp(respMap map[string]interface{}, path string) error {
	prettyJsonMsg, err := prettilize(respMap)
	if err != nil {
		return err
	}

	vid, name := respMap["id"].(string), respMap["name"].(string)
	name = strings.ReplaceAll(name, "/", "-")
	name = vid + "_" + name + ".json"
	filePath := filepath.Join(path, name)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}

	if _, err := file.Write(prettyJsonMsg); err != nil {
		return err
	}

	return nil
}
