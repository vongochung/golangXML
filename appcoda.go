package main

import (
	"encoding/xml"
	"html/template"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var (
	httpClient     *http.Client
	arrPost        []object
	postPerPage    *int
	baseURL        string
)

const (
	xmlHeader = `<?xml version="1.0" encoding="utf-8"?>`+"\n"
)

type object struct {
	Model   string 	`xml:"model,attr"`
	Category ForeignKey  `xml:"field1"`
	Author ForeignKey  `xml:"field2"`
	Title Field `xml:"field3"`
	Description Field `xml:"field4"`
	Content Field `xml:"field5"`
	Date Field `xml:"field6"`
}


type ForeignKey struct {
	To   string `xml:"to,attr"`
	Name string `xml:"name,attr"`
	Rel string `xml:"rel,attr"`
	Value    string `xml:",innerxml"`
}

type Field struct {
	Name string `xml:"name,attr"`
	Type string `xml:"type,attr"`
	Value    string `xml:",innerxml"`
}

type ListPost struct {
	XMLName xml.Name     `xml:"django-objects"`
	Version string `xml:"version,attr"`
	Objects   []object `xml:"object"`
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() * 2)
	flag.Parse()
	if flag.NArg() < 1 {
		return
	}
	u, err := url.Parse(flag.Arg(0))
	if err != nil {
		panic(err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		log.Fatal("Unrecognized URL-scheme: \"" + u.Scheme + "\".\n")
	}
	httpClient = &http.Client{}
	baseURL = u.String()
	var wg sync.WaitGroup
	CrawlerBlog(baseURL, &wg)
}

func CrawlerBlog(urlPage string, wg *sync.WaitGroup){
	d := RequestPage(urlPage)
	links := d.Find(".entry-content ul a")
	page := ""
	links.Each(func(i int, s *goquery.Selection) {
		page, _ = s.Attr("href")
		wg.Add(1)
		go GetPostOnPage(page, wg)
	})
	wg.Wait()
	CreateXML("initial_data1.xml")
	fmt.Printf("\nHoàn Thành")
	
}

// RequestPage make a http get method and return a goquery.Document
func RequestPage(pageURL string) (doc *goquery.Document) {

	resp, err := httpClient.Get(pageURL)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	d, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	return d
}


// GetPostOnPage get all Post on this page
func GetPostOnPage(urlPost string, wg *sync.WaitGroup) {
	var objectOnPage object
	d := RequestPage(urlPost)
	htmls := d.Find("#content")
	title :=  htmls.Find(".entry-title").Text()
	fmt.Printf("đã lấy: " + title + "\n")
	cate := ForeignKey{
		To: "home.category",
		Name: "category",
		Rel: "ManyToOneRel",
		Value: "5295247999369216",
	}
	auth := ForeignKey{
		To: "auth.user",
		Name: "author",
		Rel: "ManyToOneRel",
		Value: "5840605766746112",
	}
	objectOnPage.Model = "home.post"
	objectOnPage.Category = cate
	objectOnPage.Author = auth
	objectOnPage.Title = Field{
		Name : "title",
		Type : "CharField",
		Value   : title,
	}
	objectOnPage.Description = Field{
		Name : "description",
		Type : "TextField",
		Value   : title,
	}
	ct := htmls.Find(".entry-content")
	contentRemove,_:= ct.Find("#o3-social-share").Remove().Html()
	content ,_ := ct.Html()
	content = strings.TrimSpace(strings.Replace(content, contentRemove, "", -1))
	objectOnPage.Content = Field{
		Name : "content",
		Type : "TextField",
		Value   : template.HTMLEscapeString(content),
	}
    t := time.Now().Format("2006-01-02 15:04:05")
	objectOnPage.Date = Field{
		Name : "date",
		Type : "DateTimeField",
		Value   : t,
	}
	arrPost = append(arrPost, objectOnPage)
	wg.Done()
}

// CreateXML export xml file
func CreateXML(fileName string) {
	fmt.Printf("vo ==> %d", len(arrPost) )
	dataPost := &ListPost{
		Version : "1.0",
		Objects:  arrPost ,
	}

	writer, _ := os.Create(fileName)
	defer writer.Close()
	var xmlData []byte
	var err error
	xmlData, err = xml.MarshalIndent(dataPost, "", "\t")
	panicIf(err)

	content := xmlHeader + string(xmlData)
	xmlData = []byte(html.UnescapeString(content))
	_, e := writer.Write(xmlData)
	panicIf(e)
}

func panicIf(err error) {
	if err != nil {
		panic(err)
	}
}
