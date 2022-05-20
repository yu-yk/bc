package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/spf13/cast"
	"github.com/yu-yk/bc"
)

type uiInput struct {
	errLog       *widget.Entry
	ffmpegErrLog *widget.Entry
	response     *widget.Entry
	downloads    *widget.Entry
	url          *widget.Entry
	headers      *widget.Entry
	accID        *widget.Entry
	startVID     *widget.Entry
	workers      *widget.Select
	pool         *widget.Select
	step         *widget.Entry
	chkDownload  *widget.Check
	butStart     *widget.Button
	butStop      *widget.Button
}

func main() {
	app := app.New()
	w := app.NewWindow("bc")
	w.Resize(fyne.NewSize(800, 800))

	in := uiInput{}

	in.errLog = widget.NewEntry()
	in.errLog.SetPlaceHolder("File to save error log")
	in.errLog.SetText("./error.log")
	in.errLog.Validator = mustNotEmpty
	in.ffmpegErrLog = widget.NewEntry()
	in.ffmpegErrLog.SetPlaceHolder("File to save ffmpeg error log")
	in.ffmpegErrLog.SetText("./ffmpeg_error.log")
	in.ffmpegErrLog.Validator = mustNotEmpty
	in.response = widget.NewEntry()
	in.response.SetPlaceHolder("Path to save response")
	in.response.SetText("./response")
	in.response.Validator = mustNotEmpty
	in.downloads = widget.NewEntry()
	in.downloads.SetPlaceHolder("Path to save downloaded video")
	in.downloads.SetText("./downloads")
	in.downloads.Validator = mustNotEmpty
	in.url = widget.NewEntry()
	in.url.SetPlaceHolder("URL")
	in.url.Validator = mustNotEmpty
	in.headers = widget.NewMultiLineEntry()
	in.headers.SetPlaceHolder("HTTP headers in JSON format")
	in.headers.Text = `{
	"accept":"",
	"origin":"",
	"referer":""
}`
	in.headers.Validator = mustNotEmpty
	in.accID = widget.NewEntry()
	in.accID.SetPlaceHolder("Account ID")
	in.accID.Validator = mustNumber
	in.startVID = widget.NewEntry()
	in.startVID.SetPlaceHolder("Start video ID")
	in.startVID.Validator = mustNumber
	in.workers = widget.NewSelect([]string{"10", "20", "50", "100"}, nil)
	in.workers.PlaceHolder = "No. of workers"
	in.pool = widget.NewSelect([]string{"10", "20", "50", "100"}, nil)
	in.pool.PlaceHolder = "Pool size"
	in.step = widget.NewEntry()
	in.step.SetPlaceHolder("Step")
	in.step.SetText("1000")
	in.step.Validator = mustNumber
	in.chkDownload = widget.NewCheck("download", func(b bool) {})

	// for validation usage.
	uiCollections := map[string]fyne.CanvasObject{
		"ErrLog":       in.errLog,
		"FfmpegErrLog": in.ffmpegErrLog,
		"Response":     in.response,
		"Downloads":    in.downloads,
		"URL":          in.url,
		"Headers":      in.headers,
		"AccID":        in.accID,
		"StartVID":     in.startVID,
		"Workers":      in.workers,
		"Pool":         in.pool,
		"Step":         in.step,
	}

	var ctx context.Context
	var stop context.CancelFunc
	finish := make(chan struct{})
	in.butStart = widget.NewButton("Start", func() {
		if err := validateInput(uiCollections); err != nil {
			dialog.NewError(err, w).Show()
			return
		}
		// log.Println(cast.ToStringMapString(in.headers.Text))
		// log.Println(in.workers.Selected)
		// log.Println(in.pool.Selected)
		in.butStart.Disable()
		in.butStop.Enable()

		// Create a signal context to stop the API loop.
		ctx, stop = signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		go start(ctx, finish, in)
	})
	in.butStop = widget.NewButton("Stop", func() {
		in.butStop.Disable()
		stop()
		<-finish
		fmt.Println("stopped")
		in.butStart.Enable()
	})
	in.butStop.Disable()

	content := container.NewVBox(
		in.errLog, in.ffmpegErrLog, in.response, in.downloads, in.url, in.headers,
		fyne.NewContainerWithLayout(layout.NewGridLayout(2), in.accID, in.startVID),
		fyne.NewContainerWithLayout(layout.NewGridLayout(2), in.workers, in.pool),
		fyne.NewContainerWithLayout(layout.NewGridLayout(2), in.step, in.chkDownload),
		fyne.NewContainerWithLayout(layout.NewGridLayout(2), in.butStart, in.butStop),
	)

	w.SetContent(content)
	w.Show()
	app.Run()
}

func validateInput(objs map[string]fyne.CanvasObject) error {
	for name, obj := range objs {
		switch v := obj.(type) {
		case *widget.Entry:
			if err := v.Validate(); err != nil {
				return errors.New(name + " " + err.Error())
			}
		case *widget.Select:
			if v.Selected == "" {
				return errors.New(name + " not selected")
			}
		}
	}
	return nil
}

func start(ctx context.Context, finish chan struct{}, in uiInput) {
	// Create directory.
	if err := bc.Mkdir(in.downloads.Text); err != nil {
		panic(err)
	}
	if err := bc.Mkdir(in.response.Text); err != nil {
		panic(err)
	}

	// Set error log file.
	errorLogFile, err := os.OpenFile(in.errLog.Text, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	log.SetOutput(errorLogFile)

	// Set ffmpeg error log file.
	ffmpegErrLogFile, err := os.OpenFile(in.ffmpegErrLog.Text, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}

	url := in.url.Text
	accountID := in.accID.Text
	intPool, _ := strconv.Atoi(in.pool.Selected)
	intWorkers, _ := strconv.Atoi(in.workers.Selected)
	intStartVID, _ := strconv.Atoi(in.startVID.Text)
	intStep, _ := strconv.Atoi(in.step.Text)

	c := bc.Crawler{
		URL:              strings.ReplaceAll(url, ":account_id", accountID),
		Headers:          cast.ToStringMapString(in.headers.Text),
		PoolSize:         intPool,
		Workers:          intWorkers,
		StartVID:         intStartVID,
		Step:             intStep,
		Download:         in.chkDownload.Checked,
		DownloadsPath:    in.downloads.Text,
		ResponsePath:     in.response.Text,
		ErrorLogFile:     errorLogFile,
		FfmpegErrLogFile: ffmpegErrLogFile,
	}

	// fmt.Println(c.URL)
	c.Run(ctx)
	finish <- struct{}{}
	fmt.Println("struct sent!")
}

func mustNotEmpty(s string) error {
	if s == "" {
		return errors.New("is empty")
	}
	return nil
}

func mustNumber(s string) error {
	if _, err := strconv.Atoi(s); err != nil {
		return errors.New("is not a number")
	}
	return nil
}
