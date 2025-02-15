package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	session "github.com/yan00s/go-session-client"
)

type Chunk struct {
	Data []byte
	Id   int
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func readWriteChunks(nameFile string, lastChunkId int, chunkChan chan Chunk, wg *sync.WaitGroup) {
	defer wg.Done()
	var chunks []Chunk
	var needClear bool

	ticker := time.NewTicker(5 * time.Second) // Тикер для проверки каждую секунду

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				if len(chunks) > 0 {
					lastChunkId, _ = writeChunks(nameFile, chunks, lastChunkId)
					fmt.Printf("Last chunkId=%v Write remaining results...\n", lastChunkId)
				}
				fmt.Println("Channel closed, finishing...")
				return
			}
			chunks = append(chunks, chunk)
		case <-ticker.C:
			if len(chunks) == 0 {
				fmt.Println("Don't have a chunks")
				break
			}

			lastChunkId, needClear = writeChunks(nameFile, chunks, lastChunkId)
			if needClear {
				chunks = []Chunk{}
			}
			fmt.Printf("Last chunkId=%v Write results...\n", lastChunkId)
			time.Sleep(1 * time.Second)
		}
	}
}

func downloadChunk(url string, idChan chan int, chunkChan chan Chunk, wg *sync.WaitGroup) {
	defer wg.Done()
	errCountMax := 10
	curCountErr := 0
	ses := session.CreateSession(true)

	for {
		id := <-idChan
		urlChank := strings.ReplaceAll(url, "{seg}", fmt.Sprintf("%d", id))
		for {
			resp := ses.SendReq(urlChank, "GET", 10*time.Second)

			if resp.Err != nil {
				fmt.Println("ERR in response:", resp.Err.Error(), "Wait 10 seconds...")
				time.Sleep(10 * time.Second)
				continue
			}

			if strings.Contains(strings.ToLower(resp.String()), "not found") {
				fmt.Println("Goroutine finish...")
				return
			}

			if resp.Status == 429 {

				fmt.Println("len response =", len(resp.Body))
				fmt.Println("Err too many requests wait 30 seconds...")
				time.Sleep(30 * time.Second)

			} else if resp.Status != 200 {

				fmt.Println("len response =", len(resp.Body))

				if curCountErr == errCountMax {
					fmt.Println("UNKNOWN RESP:", resp.String(), "status =", resp.Status, "max errs reached, finish...")
					os.Exit(1)
				}

				fmt.Println("UNKNOWN RESP:", resp.String(), "status =", resp.Status, "Wait 8 seconds...")
				time.Sleep(8 * time.Second)
				curCountErr++
			} else {

				if len(resp.Body) < 600 {
					fmt.Println("Minimal size response reached, finish...")
					return
				}

				chunk := Chunk{Id: id, Data: resp.Body}
				chunkChan <- chunk
				break
			}
		}
	}

}

func genChunkChan(startChunkId int) chan int {
	chunCount := 999999
	result := make(chan int, chunCount)
	for i := startChunkId; i < chunCount; i++ {
		result <- i
	}
	return result
}

func writeChunks(nameFile string, chunks []Chunk, lastId int) (int, bool) {

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Id < chunks[j].Id
	})

	file, err := os.OpenFile(nameFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening file:", err)
		os.Exit(1)
	}
	defer file.Close()

	for _, chunk := range chunks {
		if lastId+1 == chunk.Id {

			if _, err := file.Write(chunk.Data); err != nil {
				fmt.Println("Error writing to file:", err)
			}

			lastId++
		} else if lastId+1 >= chunk.Id {
			continue
		}
	}

	return lastId, lastId == chunks[len(chunks)-1].Id
}

func findBetterQuality(urls []string) string {
	var longest string
	var qualities = []string{"1080", "720", "480", "240"}

	for _, quality := range qualities {
		for _, url := range urls {
			if strings.Contains(url, quality) {
				if len(longest) < len(url) {
					longest = url
				}
			}
		}
		if len(longest) > 0 {
			return longest
		}
	}
	for _, url := range urls {
		if len(longest) < len(url) {
			longest = url
		}
	}
	return longest
}

func getDomain(link string) (string, error) {
	var domain string
	parsedURL, err := url.Parse(link)
	if err != nil {
		return "", err
	}
	if len(parsedURL.Host) > 0 {
		domain = parsedURL.Scheme + "://" + parsedURL.Host
	}
	return domain, nil
}

func randomString(length int) string {
	bytes := make([]byte, length)
	for i := range bytes {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		bytes[i] = charset[n.Int64()]
	}
	return string(bytes)
}

func getNameFile(url string) string {
	var nameFile string
	splitedUrl := strings.Split(url, ".mp4")
	splitedUrl = strings.Split(splitedUrl[0], "/")

	if len(splitedUrl) <= 1 {
		nameFile = fmt.Sprintf("%v.mp4", randomString(6))
	} else {
		nameFile = fmt.Sprintf("%v.mp4", splitedUrl[len(splitedUrl)-1])
	}
	return nameFile
}

func findUrl(baseUrl, content string, needShowAll bool) (string, bool) {
	var url string
	var isChunk bool

	var schemes = []string{`["|']([^\s'"]+\.mp4.*?)["|']`, `videoUrl":"(.*?)"`} //, `(http.{1,300}\.mp4.*?)`}
	var clearUlrs []string

	for _, scheme := range schemes {
		re, err := regexp.Compile(scheme)

		if err != nil {
			log.Fatalln("Err in compile regexp", err)
		}
		resultUrls := re.FindAllStringSubmatch(content, -1)

		for _, result := range resultUrls {
			url = result[1]
			if needShowAll {
				fmt.Println(result)
			}

			if slices.Contains(clearUlrs, url) || strings.Contains(url, "gif") || strings.Contains(url, `,.`) || strings.Contains(url, "jpg") {
				continue
			}

			if strings.Contains(url, "master.m3u8") {
				url = strings.ReplaceAll(url, "master.m3u8", "seg-{seg}-v1-a1.ts")
				isChunk = true
			}

			url = strings.ReplaceAll(url, "\\", "")
			clearUlrs = append(clearUlrs, url)
		}
	}

	url = findBetterQuality(clearUlrs)
	domain, _ := getDomain(url)

	if domain == "" && url != "" {
		domain, _ = getDomain(baseUrl)
		url = fmt.Sprintf("%v%v", domain, url)
	}

	return url, isChunk
}

func getVideoUrl(baseUrl string, needShowAll bool) (string, bool) {
	var url string
	var isChunck bool

	ses := session.CreateSession(true)
	resp := ses.SendReq(baseUrl, "GET", 10*time.Second)
	if resp.Err != nil {
		log.Fatalf("Err send request on get video url: %v", resp.Err.Error())
	}

	url, isChunck = findUrl(baseUrl, resp.String(), needShowAll)

	return url, isChunck
}

func main() {
	var isChunk bool
	var url string
	baseUrl := flag.String("url", "", "URL with video (example: https://example.com/video650)")
	segUrl := flag.String("segurl", "", "segment url with video (example: https://example.com/video650-{seg}-1.ts)")
	numWorkers := flag.Int("threads", 10, "Count Threads (default: 10)")
	startChunkId := flag.Int("start", 1, "ID first chunk (default: 1)")
	needShowAll := flag.Bool("show", false, "show all coincidences urls (default: no)")

	flag.Parse()

	if *baseUrl == "" && *segUrl == "" {
		fmt.Println("Url not set!")
		flag.Usage()
		log.Fatal()
	}

	if *segUrl == "" {
		url, isChunk = getVideoUrl(*baseUrl, *needShowAll)
		if url == "" {
			log.Fatalln("Not found video direct url")
		}
	} else {
		url = *segUrl
		isChunk = true
	}

	if isChunk {
		fmt.Println("Direct url:", url)
		fmt.Println("Founded direct url, start working...")
		var wg sync.WaitGroup
		var wgRW sync.WaitGroup
		chunkChan := make(chan Chunk)
		idChan := genChunkChan(*startChunkId)

		for i := 0; i < *numWorkers; i++ {
			wg.Add(1)
			go downloadChunk(url, idChan, chunkChan, &wg)
		}

		wgRW.Add(1)
		go readWriteChunks(getNameFile(url), *startChunkId, chunkChan, &wgRW)

		wg.Wait()
		close(chunkChan)
		wgRW.Wait()
	} else {
		fmt.Println("Is not chunked, DirectUrl:", url)
	}
}
