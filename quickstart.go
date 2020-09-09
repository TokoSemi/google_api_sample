package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

const (
	credentials string = "client_id.json"
	dist        string = "dist"

	tokFile string = "token.json"
)

var removeTemporalPdf = true
var folderId string

// contains?
// return the first index if src contains elem
//        -1 otherwise
func contains(src []string, elem string) int {
	for i, v := range src {
		if v == elem {
			return i
		}
	}
	return -1
}

// inputFolderId
//    requires to input folder id from stdin
func inputFolderId() {
	sc := bufio.NewScanner(os.Stdin)
	fmt.Println("Input google folder id.")

	if sc.Scan() {
		folderId = sc.Text()
	}
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// PrintFile fetches and displays the given file.
func PrintFile(d *drive.Service, fileId string) error {
	f, err := d.Files.Get(fileId).
		Fields("parents, id, name, mimeType").Do()
	if err != nil {
		fmt.Printf("An error occurred: %v\n", err)
		return err
	}
	fmt.Printf("Name: %v ", f.Name)
	fmt.Printf("MIME type: %v ", f.MimeType)
	fmt.Printf("Parent: %v ", f.Parents)
	fmt.Println("")
	return nil
}

// Printout print data on a printer.
func Printout(data []byte) {
	pr, pw := io.Pipe()
	c := exec.Command("lpr")
	c.Stdin = pr

	go func() {
		buf := bytes.NewBuffer(data)
		buf.WriteTo(pw)
		pw.Close()
	}()

	c.Start()
	c.Wait()
}

// DownloadFile fetches and downloads the given file as mimetype pdf
func DownloadFile(d *drive.Service, fileId string, fileName string) error {
	response, err := d.Files.Export(fileId, "application/pdf").Fields("size(A4), fitw(true)").Download()
	// response, err := d.Files.Get (fileId).Download ()
	if err != nil {
		fmt.Printf("An error occurred: %v\n", err)
		return err
	}
	defer response.Body.Close()
	data, err := ioutil.ReadAll(response.Body)

	// Print on a printer
	// Printout(data)

	// Save as pdf
	ioutil.WriteFile(fileName, data, 0644)

	return nil
}

// FromSpreadsheetToPdf fetches and downloads the given spreadsheet
//     with A4 paper size
func FromSpreadsheetToPdf(file *drive.File, config *oauth2.Config) error {
	token, err := tokenFromFile("token2.json")
	if err != nil {
		token = getTokenFromWeb(config)
		saveToken("token2.json", token)
	}

	url := "https://docs.google.com/spreadsheets/d/" + file.Id + "/export?" +
		"format=pdf" +
		"&size=A4" +
		"&fitw=true" +
		"&gid=0" + // 0?
		"&access_token=" + token.AccessToken

	req, err := http.NewRequest("Get", url, nil)
	if err != nil {
		fmt.Printf("An error occurred: %v\n", err)
		return err
	}
	client := new(http.Client)
	response, _ := client.Do(req)

	defer response.Body.Close()

	fmt.Println(response.StatusCode)
	data, err := ioutil.ReadAll(response.Body)

	// Save as pdf
	fileName := dist + "/" + file.Name + ".pdf"
	ioutil.WriteFile(fileName, data, 0644)

	// Print on a printer
	// Printout(data)

	return nil
}

func main() {
	inputFolderId()

	if err := os.Mkdir(dist, 0777); err != nil {
		fmt.Printf("Unable to create directory: %v\n", err)
	}

	b, err := ioutil.ReadFile(credentials)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)
	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v", err)
	}

	r, err := srv.Files.List().PageSize(10).
		Fields("nextPageToken, files(parents, id, name, mimeType)").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve files: %v", err)
	}
	if len(r.Files) == 0 {
		fmt.Println("No files found.")
	} else {
		for _, i := range r.Files {
			if contains(i.Parents, folderId) >= 0 {
				fmt.Printf("%s (%s) %s\n", i.Name, i.Id, i.MimeType)
				err := FromSpreadsheetToPdf(i, config)
				// err := DownloadFile(srv, i.Id, dist+"/"+i.Name+".pdf")
				// err := PrintFile (srv, i.Id)
				if err != nil {
				}
			}
		}
	}
}
