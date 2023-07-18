package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var ffmpegPath string

func setLogger() {
	// open log.txt file
	f, err := os.OpenFile("error.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	// set output of logs to f
	log.SetOutput(f)
}

func main() {
	setLogger()
	// Create a new Fyne application
	app := app.New()

	// Create the main window
	window := app.NewWindow("Video Converter")
	window.Resize(fyne.NewSize(800, 800))

	// Get the current directory
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get current directory: %v", err)
	}

	// Create FFmpeg /bin/ directory path
	ffmpegPath = filepath.Join(dir, "bin")

	// Create input file selection form
	inputFileEntry := widget.NewEntry()
	inputFileButton := widget.NewButton("Select Input File", func() {
		showFileSelectionDialog(window, inputFileEntry)
	})
	inputFileForm := widget.NewForm(
		widget.NewFormItem("Input File:", inputFileEntry),
	)
	inputFileForm.Append("", inputFileButton)

	// Create output folder selection form
	outputFolderEntry := widget.NewEntry()
	outputFolderButton := widget.NewButton("Select Output Folder", func() {
		showFolderSelectionDialog(window, outputFolderEntry)
	})
	outputFolderForm := widget.NewForm(
		widget.NewFormItem("Output Folder:", outputFolderEntry),
	)
	outputFolderForm.Append("", outputFolderButton)

	// Create the convert button
	convertButton := widget.NewButton("Convert", func() {
		inputFile := inputFileEntry.Text
		outputFolder := outputFolderEntry.Text

		if inputFile == "" || outputFolder == "" {
			showErrorDialog(window, "Error", "Please select input file and output folder")
			return
		}

		// Run the video conversion
		err := convertVideo(window, inputFile, outputFolder)
		if err != nil {
			showErrorDialog(window, "Error", fmt.Sprintf("Failed to convert video: %v", err))
			return
		}

		showInfoDialog(window, "Success", "Video conversion complete")
	})

	// Create the main container
	container := container.NewVBox(
		inputFileForm,
		outputFolderForm,
		convertButton,
	)

	// Set the main container as the content of the window
	window.SetContent(container)

	// Show the window and start the Fyne application
	window.ShowAndRun()
}


func showFileSelectionDialog(parent fyne.Window, entry *widget.Entry) {
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err == nil && reader != nil {
			entry.SetText(reader.URI().Path())
		}
	}, parent)
	fileDialog.Resize(fyne.NewSize(800, 600)) // Set the desired width and height
	fileDialog.Show()
}

func showFolderSelectionDialog(parent fyne.Window, entry *widget.Entry) {
	folderDialog := dialog.NewFolderOpen(func(folder fyne.ListableURI, err error) {
		if err == nil && folder != nil {
			entry.SetText(folder.Path())
		}
	}, parent)
	folderDialog.Resize(fyne.NewSize(800, 600)) // Set the desired width and height
	folderDialog.Show()
}



func showErrorDialog(window fyne.Window, title, message string) {
	dialog.ShowError(fmt.Errorf(message), window)
}

func showInfoDialog(window fyne.Window, title, message string) {
	dialog.ShowInformation(title, message, window)
}

func getVideoDimensions(inputFile string) (dimensions string, err error) {
	// Run FFprobe command to get video information
	cmd := exec.Command(fmt.Sprintf("%s/ffprobe", ffmpegPath),
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=s=x:p=0",
		inputFile,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get video dimensions: %v", err)
	}

	dimensions = string(output)
	return dimensions, nil
}

func calculateDimensions(dimensions string) (newWidth, newHeight int) {
	var width, height int
	_, err := fmt.Sscanf(dimensions, "%dx%d", &width, &height)
	if err != nil {
		return width, height
	}

	if width > height {
		// Landscape orientation
		newWidth = 512
		newHeight = height * 512 / width
	} else {
		// Portrait or square orientation
		newWidth = width * 512 / height
		newHeight = 512
	}

	return newWidth, newHeight
}

func convertVideo(window fyne.Window, inputFile string, outputFolder string) error {
	// Check if the FFmpeg executable file exists
	if _, err := os.Stat(ffmpegPath); os.IsNotExist(err) {
		return fmt.Errorf("FFmpeg executable file does not exist")
	}

	// Check if the input file exists
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return fmt.Errorf("input file does not exist")
	}

	// Create the output folder if it doesn't exist
	err := os.MkdirAll(outputFolder, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create output folder: %v", err)
	}

	// Get the filename and extension from the input file
	filename := filepath.Base(inputFile)
	extension := filepath.Ext(filename)
	filenameWithoutExt := filename[:len(filename)-len(extension)]

	// Set the output file path
	outputFile := filepath.Join(outputFolder, filenameWithoutExt+".webm")

	// Get the video dimensions
	dimensions, err := getVideoDimensions(inputFile)
	if err != nil {
		showErrorDialog(window, "Error", fmt.Sprintf("Failed to get video dimensions: %v", err))
		return err
	}

	// Calculate the new dimensions based on the longer side being 512 pixels
	newWidth, newHeight := calculateDimensions(dimensions)

	// Run FFmpeg command to convert the video
	cmd := exec.Command(fmt.Sprintf("%s/ffmpeg", ffmpegPath),
		"-y",                                // Overwrite output file if it exists
		"-i", inputFile,                     // Input file
		"-vf", fmt.Sprintf("scale=%d:%d", newWidth, newHeight),  // Resize with calculated dimensions
		"-c:v", "libvpx-vp9",                // VP9 video codec
		"-an",                               // No audio stream
		"-r", "30",                          // Set frame rate to 30 FPS
		"-t", "3",                           // Limit duration to 3 seconds
		"-loop", "0",                        // Loop the video
		"-s", fmt.Sprintf("%dx%d", newWidth, newHeight),  // Output size with new dimensions
		"-crf", "36",                        // Control output video quality (lower value means higher quality)
		"-b:v", "256k",                      // Set maximum output bitrate to 256 Kbps
		outputFile,
	)

	// Execute the FFmpeg command
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to convert video: %v", err)
	}

	return nil
}
