package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
)

var bot *tgbotapi.BotAPI
var dir string
var token string

var compatibleFormats = []string{".mp4", ".mov", ".avi", ".mkv", ".webm"}

func isCompatible(filename string) bool {
	// check if the file is compatible with ffmpeg
	var compatible = false
	lowercaseFilePath := strings.ToLower(filename)
	for _, format := range compatibleFormats {
		if strings.HasSuffix(lowercaseFilePath, format) {
			compatible = true
			break
		}
	}
	return compatible
}

func main() {
	// Get the current directory
	exePath, err := os.Executable()
	if err != nil {
		fmt.Print("Failed to get executable path:", err)
		os.Exit(1)
	}

	dir = filepath.Dir(exePath)
	envFilePath := filepath.Join(dir, ".env")

	err = godotenv.Load(envFilePath)
	if err != nil {
		fmt.Print("Error loading .env file:", err)
		os.Exit(1)
	}

	token = os.Getenv("BOT_TOKEN")

	// Create a new Telegram bot
	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		fmt.Printf("failed to initialize Telegram bot: %v", err)
		os.Exit(1)
	}

	fmt.Println("Bot is running")

	// Register the bot command handlers
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		fmt.Printf("failed to get update channel: %v", err)
		os.Exit(1)
	}

	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Handle the "/start" command
		if update.Message.IsCommand() && update.Message.Command() == "start" {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Welcome to the Video Converter bot! Please upload a video file. This bot uses ffmpeg to convert videos.")
			// send message with url to the ffmpeg website
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonURL("FFmpeg", "https://ffmpeg.org/"),
				), tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonURL("Donate to ffmpeg", "https://ffmpeg.org/donations.html"),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonURL("Donate to me", "https://paypal.me/danielepro"),
				),
			)
			bot.Send(msg)
		}

		// Handle document uploads (videos)
		if update.Message.Document != nil || update.Message.Video != nil {
			var fileID = ""
			var filePath = ""
			if update.Message.Document != nil {
				fileID = update.Message.Document.FileID
				filePath = update.Message.Document.FileName
			} else if update.Message.Video != nil {
				fileID = update.Message.Video.FileID
				filePath = fmt.Sprintf("%s.mp4", update.Message.Video.FileID)
			}

			// check if the file is compatible with ffmpeg
			var compatible = isCompatible(filePath)
			if !compatible {
				sendMessage(update.Message.Chat.ID, "File format not supported.")
				continue
			}

			// send "converting" message
			sendMessage(update.Message.Chat.ID, "Converting video...")

			// Download the file
			joinedFilePath := filepath.Join(dir, filePath)
			err := downloadFile(fileID, joinedFilePath)
			if err != nil {
				sendMessage(update.Message.Chat.ID, "Failed to download file.")
				fmt.Printf("failed to download file: %v", err)
				continue
			}

			// Convert the video
			convertedFilePath, err := convertVideo(joinedFilePath)
			if err != nil {
				sendMessage(update.Message.Chat.ID, "Failed to convert video.")
				fmt.Printf("failed to convert video: %v", err)
				os.Remove(joinedFilePath)
				os.Remove(convertedFilePath)
				continue
			}
			sendMessage(update.Message.Chat.ID, "Video converted, sending back...")
			// Send the converted video back
			video := tgbotapi.NewVideoUpload(update.Message.Chat.ID, convertedFilePath)
			bot.Send(video)
			os.Remove(joinedFilePath)
			os.Remove(convertedFilePath)
		}
	}
}

func sendMessage(chatID int64, text string) {
	go func() {
		msg := tgbotapi.NewMessage(chatID, text)
		_, err := bot.Send(msg)
		if err != nil {
			fmt.Printf("failed to send message: %v", err)
		}
	}()
}

func downloadFile(fileID, filePath string) error {
	fileConfig := tgbotapi.FileConfig{
		FileID: fileID,
	}

	file, err := bot.GetFile(fileConfig)
	if err != nil {
		return fmt.Errorf("failed to get file: %v", err)
	}

	resp, err := http.Get(file.Link(bot.Token))
	if err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	output, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer output.Close()

	_, err = io.Copy(output, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %v", err)
	}

	return nil
}

func convertVideo(inputFile string) (string, error) {
	// Check if the FFmpeg is installed on linux
	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg is not installed")
	}

	// Check if the input file exists
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return "", fmt.Errorf("input file does not exist")
	}

	// Get the filename and extension from the input file
	filename := filepath.Base(inputFile)
	extension := filepath.Ext(filename)
	filenameWithoutExt := filename[:len(filename)-len(extension)]

	// Set the output file path
	outputFile := filepath.Join(".", filenameWithoutExt+".webm")

	// Get the video dimensions
	dimensions, err := getVideoDimensions(inputFile)
	if err != nil {
		return "", fmt.Errorf("failed to get video dimensions: %v", err)
	}

	// Calculate the new dimensions based on the longer side being 512 pixels
	newWidth, newHeight := calculateDimensions(dimensions)

	// Run FFmpeg command to convert the video
	cmd := exec.Command("ffmpeg",
		"-y",            // Overwrite output file if it exists
		"-i", inputFile, // Input file
		"-vf", fmt.Sprintf("scale=%d:%d", newWidth, newHeight), // Resize with calculated dimensions
		"-c:v", "libvpx-vp9", // VP9 video codec
		"-an",      // No audio stream
		"-r", "30", // Set frame rate to 30 FPS
		"-t", "3", // Limit duration to 3 seconds
		"-loop", "0", // Loop the video
		"-s", fmt.Sprintf("%dx%d", newWidth, newHeight), // Output size with new dimensions
		"-crf", "36", // Control output video quality (lower value means higher quality)
		"-b:v", "256k", // Set maximum output bitrate to 256 Kbps
		outputFile,
	)

	// Execute the FFmpeg command
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to convert video: %v", err)
	}

	return outputFile, nil
}

func getVideoDimensions(inputFile string) (dimensions string, err error) {
	// Run FFprobe command to get video information
	cmd := exec.Command("ffprobe",
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
