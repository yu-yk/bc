package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/spf13/cast"
	"github.com/yu-yk/bc"
)

type uiInput struct {
	errLog        *widget.Entry
	ffmpegErrLog  *widget.Entry
	response      *widget.Entry
	downloads     *widget.Entry
	url           *widget.Entry
	headers       *widget.Entry
	accID         *widget.Entry
	startVID      *widget.Entry
	workers       *widget.Select
	pool          *widget.Select
	step          *widget.Entry
	chkDownload   *widget.Check
	butStart      *widget.Button
	butStop       *widget.Button
	currVIDStr    *bindingStr
	currStatusStr *bindingStr
}

type bindingStr struct {
	binding.String
}

func main() {
	app := app.NewWithID("bgui.yk.yu")
	w := app.NewWindow("bc")
	w.Resize(fyne.NewSize(800, 600))

	in := uiInput{}
	in.errLog = widget.NewEntry()
	in.errLog.SetPlaceHolder("File to save error log")
	in.errLog.SetText(app.Preferences().StringWithFallback("errLog", "./error.log"))
	in.errLog.Validator = mustNotEmpty
	in.ffmpegErrLog = widget.NewEntry()
	in.ffmpegErrLog.SetPlaceHolder("File to save ffmpeg error log")
	in.ffmpegErrLog.SetText(app.Preferences().StringWithFallback("ffmpegErrLog", "./ffmpeg_error.log"))
	in.ffmpegErrLog.Validator = mustNotEmpty
	in.response = widget.NewEntry()
	in.response.SetPlaceHolder("Path to save response")
	in.response.SetText(app.Preferences().StringWithFallback("response", "./response"))
	in.response.Validator = mustNotEmpty
	in.downloads = widget.NewEntry()
	in.downloads.SetPlaceHolder("Path to save downloaded video")
	in.downloads.SetText(app.Preferences().StringWithFallback("downloads", "./downloads"))
	in.downloads.Validator = mustNotEmpty
	in.url = widget.NewEntry()
	in.url.SetPlaceHolder("URL")
	in.url.SetText(app.Preferences().StringWithFallback("url", ""))
	in.url.Validator = mustNotEmpty
	in.headers = widget.NewMultiLineEntry()
	in.headers.SetPlaceHolder("HTTP headers in JSON format")
	in.headers.SetText(app.Preferences().StringWithFallback("headers", `{
	"accept":"",
	"origin":"",
	"referer":""
}`))
	in.headers.Validator = mustNotEmpty
	in.accID = widget.NewEntry()
	in.accID.SetPlaceHolder("Account ID")
	in.accID.SetText(app.Preferences().StringWithFallback("accID", ""))
	in.accID.Validator = mustNumber
	in.startVID = widget.NewEntry()
	in.startVID.SetPlaceHolder("Start video ID")
	in.startVID.SetText(app.Preferences().StringWithFallback("startVID", ""))
	in.startVID.Validator = mustNumber
	in.workers = widget.NewSelect([]string{"10", "20", "50", "100"}, nil)
	in.workers.PlaceHolder = "No. of workers"
	in.pool = widget.NewSelect([]string{"10", "20", "50", "100"}, nil)
	in.pool.PlaceHolder = "Pool size"
	in.step = widget.NewEntry()
	in.step.SetPlaceHolder("Step")
	in.step.SetText(app.Preferences().StringWithFallback("step", "1000"))
	in.step.Validator = mustNumber
	in.chkDownload = widget.NewCheck("download video", func(b bool) {})
	in.currVIDStr = &bindingStr{binding.NewString()}
	in.currVIDStr.Set(app.Preferences().StringWithFallback("currVIDStr", ""))
	in.currStatusStr = &bindingStr{binding.NewString()}
	in.currStatusStr.Set(app.Preferences().StringWithFallback("currStatusStr", ""))
	pBar := widget.NewProgressBarInfinite()
	pBar.Stop()

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
		// Simple validation for all the input
		if err := validateInput(uiCollections); err != nil {
			dialog.NewError(err, w).Show()
			return
		}

		in.butStart.Disable()
		in.butStop.Enable()
		pBar.Start()

		// Create a signal context to stop the API loop.
		ctx, stop = signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		go start(ctx, finish, in)
	})
	in.butStop = widget.NewButton("Stop", func() {
		in.butStop.Disable()
		stop()
		<-finish
		pBar.Stop()
		in.butStart.Enable()
		app.Preferences().SetString("errLog", in.errLog.Text)
		app.Preferences().SetString("ffmpegErrLog", in.ffmpegErrLog.Text)
		app.Preferences().SetString("response", in.response.Text)
		app.Preferences().SetString("downloads", in.downloads.Text)
		app.Preferences().SetString("url", in.url.Text)
		app.Preferences().SetString("headers", in.headers.Text)
		app.Preferences().SetString("accID", in.accID.Text)
		app.Preferences().SetString("startVID", in.startVID.Text)
		app.Preferences().SetString("step", in.step.Text)
		currVIDText, _ := in.currVIDStr.Get()
		app.Preferences().SetString("currVIDStr", currVIDText)
		currStatusText, _ := in.currStatusStr.Get()
		app.Preferences().SetString("currStatusStr", currStatusText)
	})
	in.butStop.Disable()

	currVID := widget.NewLabelWithData(in.currVIDStr)
	currStatus := widget.NewLabelWithData(in.currStatusStr)

	content := container.NewVBox(
		in.errLog, in.ffmpegErrLog, in.response, in.downloads, in.url, in.headers,
		fyne.NewContainerWithLayout(layout.NewGridLayout(2), in.accID, in.startVID),
		fyne.NewContainerWithLayout(layout.NewGridLayout(2), in.workers, in.pool),
		fyne.NewContainerWithLayout(layout.NewGridLayout(2), in.step, in.chkDownload),
		fyne.NewContainerWithLayout(layout.NewGridLayout(2), in.butStart, in.butStop),
		pBar,
		fyne.NewContainerWithLayout(layout.NewGridLayout(3), widget.NewLabel("current VID:"), currVID,
			widget.NewButtonWithIcon("copy vid", theme.ContentCopyIcon(), func() {
				w.Clipboard().SetContent(currVID.Text)
			})),
		fyne.NewContainerWithLayout(layout.NewGridLayout(3), widget.NewLabel("status:"), currStatus),
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
		CurrVID:          in.currVIDStr,
		CurrStatus:       in.currStatusStr,
	}

	c.Run(ctx)
	finish <- struct{}{}
}

func (bs *bindingStr) Write(p []byte) (n int, err error) {
	if err := bs.Set(string(p)); err != nil {
		return 0, nil
	}
	return len(p), nil
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
