package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

var (
	httpClient *http.Client
)

const (
	xmlHeader = `<rss xmlns:excerpt="http://wordpress.org/export/1.2/excerpt/"` +
		`xmlns:content="http://purl.org/rss/1.0/modules/content/"` +
		`xmlns:wfw="http://wellformedweb.org/CommentAPI/"` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		`xmlns:wp="http://wordpress.org/export/1.2/" version="2.0">` + "\n"
	xmlFooter = `</rss>`
)

type xmlType struct {
	XMLName xml.Name `xml:"rss"`
	Channel attachChannel
}

type attachChannel struct {
	XMLName xml.Name     `xml:"channel"`
	Version string       `xml:"wxr_version"`
	Items   []attachment `xml:"item"`
}

type attachment struct {
	Title         string `xml:"title"`
	PostType      string `xml:"post_type"`
	AttachmentURL string `xml:"attachment_url"`
}

// RequestPage make a http get method and return a goquery.Document
func getImageFromUrl(imageURL string) (l int64, err error) {

	resp, err := httpClient.Get(imageURL)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	sr := strings.SplitAfter(imageURL, "/")
	imageName := sr[len(sr)-1]
	out, _ := os.Create("image/" + imageName)
	n, err := io.Copy(out, resp.Body)
	return n, err
}

func main() {
	httpClient = &http.Client{}

	xmlFile, err := os.Open("attachment1.xml")
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer xmlFile.Close()
	data, _ := ioutil.ReadAll(xmlFile)

	var q xmlType

	xml.Unmarshal(data, &q)
	fmt.Println(q)

	//getImageFromUrl("http://linnsees.se.formecdn.com/2014/06/10013-fotor062111393.png")
}
