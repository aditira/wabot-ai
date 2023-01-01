package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"

	goopenai "github.com/CasualCodersProjects/gopenai"
	"github.com/CasualCodersProjects/gopenai/types"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

var client *whatsmeow.Client
var ai bool = false

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if !v.Info.IsFromMe {
			if v.Message.GetConversation() != "" {
				godotenv.Load(".env")
				openAI := goopenai.NewOpenAI(&goopenai.OpenAIOpts{
					APIKey: os.Getenv("AI_KEY"),
				})

				fmt.Println("PESAN DITERIMA!", v.Message.GetConversation())

				if strings.ToLower(v.Message.GetConversation()) == "/ai" {
					client.SendMessage(v.Info.Sender, "", &waProto.Message{
						Conversation: proto.String("AI: Okay I'm listening, how can I help you now?"),
					})

					ai = true
					return
				}

				if ai {
					request := types.NewDefaultCompletionRequest("The following is a conversation with an AI assistant. The assistant is helpful, creative, clever, and very friendly.\n\nHuman: Hello, who are you?\nAI: I am an AI created by OpenAI. How can I help you today?\nHuman: " + v.Message.GetConversation() + "\nAI:")
					request.Model = "text-davinci-003"
					request.Temperature = 0.9
					request.MaxTokens = 150
					request.TopP = 1
					request.FrequencyPenalty = 0
					request.PresencePenalty = 0.6
					request.Stop = []string{" Human:", " AI:"}

					resp, err := openAI.CreateCompletion(request)
					if err != nil {
						client.SendMessage(v.Info.Sender, "", &waProto.Message{
							Conversation: proto.String("AI [Automatic Messages] Error: " + err.Error()),
						})
					}

					if len(resp.Choices) == 0 {
						client.SendMessage(v.Info.Sender, "", &waProto.Message{
							Conversation: proto.String("AI [Automatic Messages] Error: " + err.Error()),
						})
					}

					client.SendMessage(v.Info.Sender, "", &waProto.Message{
						Conversation: proto.String("AI [Automatic Messages]: " + resp.Choices[0].Text),
					})

					ai = false
					return
				}

			}
		}
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/wa-start", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
		}

		dbLog := waLog.Stdout("Database", "DEBUG", true)
		// Make sure you add appropriate DB connector imports, e.g. github.com/mattn/go-sqlite3 for SQLite
		container, err := sqlstore.New("sqlite3", "file:wa.db?_foreign_keys=on", dbLog)
		if err != nil {
			panic(err)
		}
		// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
		deviceStore, err := container.GetFirstDevice()
		if err != nil {
			panic(err)
		}
		clientLog := waLog.Stdout("Client", "DEBUG", true)
		client = whatsmeow.NewClient(deviceStore, clientLog)
		client.AddEventHandler(eventHandler)

		if client.Store.ID == nil {
			// No ID stored, new login
			qrChan, _ := client.GetQRChannel(context.Background())
			err = client.Connect()
			if err != nil {
				panic(err)
			}
			for evt := range qrChan {
				if evt.Event == "code" {
					// Render the QR code here
					// e.g. qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
					// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
					fmt.Println("QR code:", evt.Code)
					w.Write([]byte("Please Scan QR-Code to Login!"))
					qrterminal.Generate(evt.Code, qrterminal.L, os.Stdout)
				} else {
					fmt.Println("Login event:", evt.Event)
				}
			}
		} else {
			// Already logged in, just connect
			w.Write([]byte("Already Logged"))

			err = client.Connect()
			if err != nil {
				panic(err)
			}
		}

		// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c

		client.Disconnect()

		t.ExecuteTemplate(w, "index.html.tmpl", data)
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
