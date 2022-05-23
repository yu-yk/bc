package bc

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

var (
	running   = "running..."
	waiting   = "waiting download tasks finish..."
	completed = "all download tasks completed."
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
	ErrorLogFile     io.WriteCloser
	FfmpegErrLogFile io.WriteCloser
	CurrVID          io.Writer
	CurrStatus       io.Writer
}

func (c *Crawler) Run(ctx context.Context) {
	// Set Output
	log.SetOutput(c.ErrorLogFile)

	// Create the pool.
	buffVIDs := make(chan string, c.PoolSize)

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
				c.CurrVID.Write([]byte(vid + " "))
				url := c.URL + vid
				resp, err := client.R().Get(url)
				if err != nil {
					log.Println(url)
					log.Println(err)
				}

				if resp.StatusCode() == http.StatusOK { // Video found.
					wg.Add(1)

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
				wg.Done()
			}
		}()
	}

	c.CurrStatus.Write([]byte(running + "\n"))

LOOP:
	for {
		select {
		case <-ctx.Done(): // Stop the loop and exit the goroutine.
			close(buffVIDs)
			break LOOP
		default:
			c.StartVID = c.StartVID + c.Step
			vid := strconv.Itoa(c.StartVID)
			buffVIDs <- vid // Fill the pool.
			wg.Add(1)
		}
	}

	c.CurrStatus.Write([]byte(waiting + "\n"))
	wg.Wait()
	c.CurrStatus.Write([]byte(completed + "\n"))
	c.ErrorLogFile.Close()
	c.FfmpegErrLogFile.Close()
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
