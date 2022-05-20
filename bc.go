package bcrawl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-resty/resty/v2"
)

type Crawler struct {
	URL              string
	Headers          map[string]string
	PoolSize         int
	Workers          int
	StartVID         int
	Step             int
	Download         bool
	ResponsePath     string
	DownloadsPath    string
	ErrorLogFile     *os.File
	FfmpegErrLogFile *os.File
}

func (c *Crawler) Run() {
	// Create the pool.
	buffVIDs := make(chan string, c.PoolSize)

	// Handle sigterm and await termChan signal.
	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGTERM, syscall.SIGINT)

	// Create wait group for background download process.
	var wg sync.WaitGroup

	// Create http client.
	client := resty.New()
	client.SetHeaders(c.Headers)
	client.SetRetryCount(3)
	client.AddRetryCondition(func(r *resty.Response, err error) bool {
		return r.StatusCode() == http.StatusGatewayTimeout
	})
	client.SetTimeout(time.Second * 5)

	// Create workers for calling the API and futher processing.
	for w := 0; w < c.Workers; w++ {
		go func() {
			// Get the vid from pool and call the API.
			for vid := range buffVIDs {
				fmt.Print(vid, " ")
				url := c.URL + vid
				resp, err := client.R().Get(url)
				if err != nil {
					log.Println(url)
					log.Println(err)
				}

				if resp.StatusCode() == http.StatusOK { // Video found.
					wg.Add(1)
					fmt.Println("found! downloading...")

					go func() { // Download the response and video concurrently.
						defer wg.Done()

						var respMap map[string]interface{}
						if err := json.Unmarshal(resp.Body(), &respMap); err != nil {
							log.Println(url)
							log.Println(err)
							return
						}

						// Save the response.
						if err := saveResp(respMap, c.ResponsePath); err != nil {
							log.Println(url)
							log.Println(err)
							return
						}

						// Download the video using ffmpeg.
						if c.Download {
							if err := saveVideo(respMap, c.DownloadsPath, c.FfmpegErrLogFile); err != nil {
								log.Println(url)
								log.Println(err)
							}
						}
					}()
				} else if resp.StatusCode() != http.StatusNotFound {
					// Log other HTTP error.
					log.Println(url)
					log.Println(resp.StatusCode())
					log.Println(string(resp.Body()))
				}
			}
		}()
	}

	// Create a signal context to stop the API loop.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done(): // Stop the loop and exit the goroutine.
				fmt.Println("video id loop stopped")
				close(buffVIDs)
				return
			default:
				c.StartVID = c.StartVID + c.Step
				vid := strconv.Itoa(c.StartVID)
				buffVIDs <- vid // Fill the pool.
			}
		}
	}(ctx)

	<-termChan // Blocks here until interrupted.
	fmt.Println("SIGTERM received. Shutdown process initiated")
	fmt.Println("waiting for running jobs to finish...")
	wg.Wait() // Wait all the download operations finish before quit.
	fmt.Println("all download completed. exiting...")
	c.ErrorLogFile.Close()
	c.FfmpegErrLogFile.Close()
}

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

// Add indentation to the json string.
func prettilize(data interface{}) ([]byte, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}
	return b, nil
}

func Mkdir(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(path, 0764); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}
