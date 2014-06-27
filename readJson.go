package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/goodsign/monday"
)

var (
	httpClient     *http.Client
	arrPost        []post
	postPerPage    *int
	baseURL        string
	arrAttachment  []attachment
	attachmentPath *string
)

const (
	day       = 86400000000000 // 1 day = 86400000000000 nano seconds
	xmlHeader = `<rss xmlns:excerpt="http://wordpress.org/export/1.2/excerpt/"` +
		`xmlns:content="http://purl.org/rss/1.0/modules/content/"` +
		`xmlns:wfw="http://wellformedweb.org/CommentAPI/"` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		`xmlns:wp="http://wordpress.org/export/1.2/" version="2.0">` + "\n"
	xmlFooter  = `</rss>`
	urlComment = `http://linnsees.forme.se/js.proxy.php?comments/get&format=json&v=2&account_key=9a22d356861ddb5b55681c76b4bd701392d151bc&content_id=`
)

type post struct {
	Title    string    `xml:"title"`
	PubDate  string    `xml:"pubDate"`
	Content  string    `xml:"content:encoded"`
	Author   string    `xml:"dc:creator"`
	PostType string    `xml:"wp:post_type"`
	PostDate string    `xml:"wp:post_date"`
	Status   string    `xml:"wp:status"`
	Category category  `xml:"category"`
	Comment  []comment `xml:"wp:comment"`
}

type comment struct {
	ID          int64  `xml:"wp:comment_id"`
	Author      string `xml:"wp:comment_author"`
	Content     string `xml:"wp:comment_content"`
	Approved    string `xml:"wp:comment_approved"`
	CommentDate string `xml:"wp:comment_date"`
}

type category struct {
	Domain   string `xml:"domain,attr"`
	Nicename string `xml:"nicename,attr"`
	Value    string `xml:",innerxml"`
}

type channel struct {
	Version string `xml:"wp:wxr_version"`
	Items   []post `xml:"item"`
}

type attachChannel struct {
	XMLName xml.Name     `xml:"channel"`
	Version string       `xml:"wp:wxr_version"`
	Items   []attachment `xml:"item"`
}

type attachment struct {
	Title         string `xml:"title"`
	PostType      string `xml:"wp:post_type"`
	AttachmentURL string `xml:"wp:attachment_url"`
}

// CommentReturn  is returned from url
type CommentReturn struct {
	Status string
	Data   commentJSON
}

type commentJSON struct {
	Comments []commentData
	Account  string
	Page     page
	Count    int
}

type commentData struct {
	ID             int
	Date           string
	Poster         poster
	Homepage       string
	Comment        string
	Status         string
	IP             string
	PageID         int
	Vote           vote
	ContentRequest contentRequest
	RawDate        string
}

type page struct {
	ID           int
	LastComment  string
	NrOfComments int
}

type poster struct {
	Name     string
	IsMember bool
}

type vote struct {
	Score int
	Plus  int
	Minus int
}

type contentRequest struct {
	Controller string
	Action     string
	Method     string
	Payload    load
}

type load struct {
	Post postload
}

type postload struct {
	ID string
}

func main() {
	postPerPage = flag.Int("post_per_page", 10, "Number of POST per Page")
	attachmentPath = flag.String("attachment", "/wp-content/uploads", "Path serve file upload")
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
	CrawlerBlog(baseURL)
}

// CrawlerBlog get blog
func CrawlerBlog(urlRoot string) {
	endPage := make(chan bool)
	nextPage := make(chan string)
	crawlerOnPage := make(chan []post)

	d := RequestPage(urlRoot)
	go GetNextPage(nextPage, endPage, d)
	go GetPostOnPage(crawlerOnPage, d)
	i := 1
	for {

		select {
		case urlNext := <-nextPage:
			{
				doc := RequestPage(urlNext)
				go GetNextPage(nextPage, endPage, doc)
				go GetPostOnPage(crawlerOnPage, doc)
			}

		case res := <-crawlerOnPage:
			arrPost = append(arrPost, res...)
			if (len(arrPost) > (i * *postPerPage)) && (len(arrPost[(i-1)**postPerPage:]) >= *postPerPage) {
				CreateXML("forme-"+strconv.Itoa(i)+".xml", (i-1)**postPerPage, i**postPerPage, false)
				i++

			}
		case <-endPage:
			{
				CreateXML("forme-end.xml", (i-1)**postPerPage, -1, false)
				CreateXML("attachment.xml", 0, -1, true)
				return
			}

		}

	}

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

// GetNextPage return url next page to crawl
func GetNextPage(nextPage chan string, endPage chan bool, d *goquery.Document) {
	var nextLink string
	d.Find(".paging a").Each(func(i int, s *goquery.Selection) {
		nextLink, _ = s.Attr("href")
	})
	u, err := url.Parse(baseURL + nextLink)
	if err != nil {
		panic(err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		log.Fatal("Unrecognized URL-scheme: \"" + u.Scheme + "\".\n")
	}
	doc := RequestPage(u.String())
	var strNext string
	doc.Find(".paging a").Each(func(i int, s *goquery.Selection) {
		strNext, _ = s.Attr("href")
	})
	fmt.Println("Crawling... => " + u.String())
	if strNext != "#top" {
		nextPage <- u.String()
	} else {
		endPage <- true
	}

}

// GetPostOnPage get all Post on this page
func GetPostOnPage(crawlerOnPage chan []post, d *goquery.Document) {
	var arrPostOnPage []post
	if d.Find(".content .posts > div").Length() != 0 {
		d.Find(".content .posts > div").Each(func(i int, s *goquery.Selection) {
			isPost, _ := s.Attr("data-like")
			if isPost == "post" {
				title := s.Find("h2").Text()
				categoryName := ""
				categories := s.Find(".footer a")
				categoryName = categories.FilterNodes(categories.Get(0)).Text()
				content, _ := s.Find(".clearfix").Html()
				pubDate := strings.Trim(s.Find(".footer .date").Text(), " ")
				var postDate string
				if len(pubDate) > 0 {
					t := findReleaseDateString(pubDate)
					pubDate = t.Format(time.RFC1123Z)
					postDate = t.Format("2006-01-02 15:04:05")
				}

				var comments []comment

				footer := s.Find(".footer a")
				var indexComment = 1
				if footer.Length() > 3 {
					indexComment = 2
				}
				idPost, _ := footer.FilterNodes(footer.Get(indexComment)).Attr("id")
				var reNum = regexp.MustCompile(`\D+`)
				if reNum.MatchString(idPost) {
					idPost = reNum.ReplaceAllString(idPost, "")
				}
				comments = GetCommentByPost(getURLComment(idPost))

				s.Find(".clearfix img").Each(func(index int, img *goquery.Selection) {
					src, _ := img.Attr("src")
					if strings.HasPrefix(src, "/") {
						content = replaceImgToAbsolutePath(src, content)
						src = baseURL + src
					}
					if !strings.HasPrefix(src, "data:image") {
						_, imageName := addAttachmentImage(src)
						content = replaceImgToServerName(src, imageName, content)
					}

				})

				if checkExistTitle(title, arrPost) {
					title = title + " " + pubDate
				}

				categoryPost := category{
					Domain:   "category",
					Nicename: html.UnescapeString(strings.ToLower(categoryName)),
					Value:    html.UnescapeString(categoryName),
				}

				onePost := post{
					Title:    html.UnescapeString(title),
					Author:   "admin",
					Content:  html.UnescapeString(content),
					PubDate:  pubDate,
					PostDate: postDate,
					PostType: "post",
					Status:   "publish",
					Category: categoryPost,
					Comment:  comments,
				}
				arrPostOnPage = append(arrPostOnPage, onePost)
			}
		})
	}

	crawlerOnPage <- arrPostOnPage

}

// CreateXML export xml file
func CreateXML(fileName string, start int, end int, isAttachment bool) {

	var items []post
	var itemsAttach []attachment
	var dataPost *channel
	var dataAttach *attachChannel
	if end < 0 {
		items = arrPost[start:]
		itemsAttach = arrAttachment[start:]
	} else {
		items = arrPost[start:end]
		itemsAttach = arrAttachment[start:end]
	}
	if isAttachment {
		dataAttach = &attachChannel{
			Version: "1.2",
			Items:   itemsAttach,
		}
	} else {
		dataPost = &channel{
			Version: "1.2",
			Items:   items,
		}
	}

	writer, _ := os.Create(fileName)
	defer writer.Close()
	var xmlData []byte
	var err error
	if isAttachment {
		xmlData, err = xml.MarshalIndent(dataAttach, "", "\t")
	} else {
		xmlData, err = xml.MarshalIndent(dataPost, "", "\t")
	}
	panicIf(err)

	content := xmlHeader + string(xmlData) + "\n" + xmlFooter
	xmlData = []byte(html.UnescapeString(content))
	_, e := writer.Write(xmlData)
	panicIf(e)
}

// GetCommentByPost get all comments on this post if it have comment
func GetCommentByPost(commentLink string) []comment {
	var comments []comment

	resp, err := httpClient.Get(commentLink)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	var data CommentReturn
	body, _ := ioutil.ReadAll(resp.Body)
	if e := json.Unmarshal(body, &data); err != nil {
		panic(e)
	}
	if len(data.Data.Comments) > 0 {
		for i := 0; i < len(data.Data.Comments); i++ {
			content := data.Data.Comments[i].Comment
			author := data.Data.Comments[i].Poster.Name
			date := data.Data.Comments[i].Date
			t, _ := time.Parse(time.RFC3339, date)
			status := data.Data.Comments[i].Status
			var Approved = 1
			if status != "approved" {
				Approved = 0
			}

			oneComment := comment{
				ID:          time.Now().Unix() + int64(i),
				Author:      author,
				Content:     html.UnescapeString(content),
				Approved:    strconv.Itoa(Approved),
				CommentDate: t.Format("2006-01-02 15:04:05"),
			}
			comments = append(comments, oneComment)
		}
	}
	return comments

}

func findReleaseDateString(raw string) time.Time {
	var t time.Time
	var err error
	loc, _ := time.LoadLocation("Europe/Stockholm")
	arrDay := strings.Split(raw, " ")
	var datetime time.Time
	if strings.Contains(arrDay[0], "Idag") {
		datetime = time.Now()
		hour := arrDay[1]
		date := strconv.Itoa(datetime.Day())
		month := datetime.Month().String()
		year := strconv.Itoa(datetime.Year())
		raw = date + " " + month + ", " + year + " " + hour
	} else if strings.Contains(arrDay[0], "Igår") {
		datetime = time.Now().Add(-day)
		hour := arrDay[1]
		date := strconv.Itoa(datetime.Day())
		month := datetime.Month().String()
		year := strconv.Itoa(datetime.Year())
		raw = date + " " + month + ", " + year + " " + hour
	} else if strings.Contains("Måndags Tisdags Onsdags Torsdags Fredags Lördags Söndags", arrDay[0]) {
		datetime = convertDayToDatetime(arrDay[0])
		hour := arrDay[1]
		date := strconv.Itoa(datetime.Day())
		month := datetime.Month().String()
		year := strconv.Itoa(datetime.Year())
		raw = date + " " + month + ", " + year + " " + hour
	}

	t, err = monday.ParseInLocation("2 January, 2006 15:04", raw, loc, monday.LocaleSvSE)

	if err != nil {
		panic(err)
	}
	return t
}

func convertDayToDatetime(strDay string) time.Time {
	switch strDay {
	case "Måndags":
		strDay = time.Monday.String()
	case "Tisdags":
		strDay = time.Tuesday.String()
	case "Onsdags":
		strDay = time.Wednesday.String()
	case "Torsdags":
		strDay = time.Thursday.String()
	case "Fredags":
		strDay = time.Friday.String()
	case "Lördags":
		strDay = time.Saturday.String()
	case "Söndags":
		strDay = time.Sunday.String()
	}
	count := 1
	for {
		date := time.Now().Add((time.Duration)(-day * count))
		if date.Weekday().String() == strDay {
			return date
		}
		count++
	}
}

func checkExistTitle(title string, arrDest []post) bool {
	for i := 0; i < len(arrDest); i++ {
		if title == arrDest[i].Title {
			return true
		}
	}

	return false
}

func checkExistImage(imageName string, arrDest []attachment) bool {
	for i := 0; i < len(arrDest); i++ {
		if imageName == arrDest[i].Title {
			return true
		}
	}

	return false
}

func replaceImgToAbsolutePath(src string, source string) string {
	var srcOld = src
	src = strings.Replace(src, "/", `\/`, -1)
	var re = regexp.MustCompile(`src=[\"']` + src)
	if re.MatchString(source) {
		source = re.ReplaceAllString(source, `src="`+baseURL+srcOld)
	}
	return source

}

func replaceImgToServerName(imageSRC string, imageName string, source string) string {
	imageSRC = strings.Split(imageSRC, "?")[0]
	imageSRC = strings.Replace(imageSRC, "/", `\/`, -1)
	var Year = strconv.Itoa(time.Now().Year())
	var Month = time.Now().Format("01")
	var serverURL = *attachmentPath + "/" + Year + "/" + Month + "/" + imageName
	var re = regexp.MustCompile(`src=[\"']` + imageSRC)
	if re.MatchString(source) {
		source = re.ReplaceAllString(source, `src="`+serverURL)
	}
	return source

}

func addAttachmentImage(src string) (newName string, rootName string) {
	sr := strings.SplitAfter(src, "/")
	if len(sr) == 0 {
		panic(sr)
	}
	imageName := strings.Split(sr[len(sr)-1], "?")[0]
	imageNameRoot := imageName
	if checkExistImage(imageName, arrAttachment) {
		t := time.Now().Local()
		imageName = t.Format("20060102150405") + "_" + imageName
	}
	imgAttachment := attachment{
		Title:         imageName,
		PostType:      "attachment",
		AttachmentURL: strings.Split(src, "?")[0],
	}
	arrAttachment = append(arrAttachment, imgAttachment)
	return imageName, imageNameRoot
}

func getURLComment(id string) string {
	return urlComment + id
}

func panicIf(err error) {
	if err != nil {
		panic(err)
	}
}
