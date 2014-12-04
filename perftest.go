package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	//"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

type Args struct {
	concurrency  int
	queryCnt     int
	serviceAddr  string
	testDataPath string
}

type Param struct {
	data     []string
	task     []int
	queryCnt int
}

type Stat struct {
	failed          int
	successed       int
	respTimeMsTotal int64
	respTimeAvgMs   int64
	respTimeMsMax   int64
}

func loadData(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	lines := make([]string, 16)

	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			if len(line) > 0 {
				lines = append(lines, string(line))
			}
			break
		}
		if err != nil {
			return nil, err
		}
		if len(line) > 0 {
			lines = append(lines, string(line))
		}
	}
	return lines, nil
}

func generateTask(cnt, max int) []int {
	ret := make([]int, cnt)
	r := rand.New(rand.NewSource(int64(time.Now().UnixNano())))
	for i := 0; i < cnt; i++ {
		ret[i] = r.Int() % max
	}
	return ret
}

func work(addr string, param *Param, stat *Stat, wc chan<- int) {
	defer func() {
		wc <- 1
		if stat.failed > 0 || stat.successed > 0 {
			stat.respTimeAvgMs = stat.respTimeMsTotal / int64(stat.failed+stat.successed)
		}
	}()

	for i := 0; i < param.queryCnt; i++ {

		url := addr
		vars := strings.TrimSpace(param.data[param.task[i]])
		if vars != "" {
			url += "?" + vars
		}

		start := time.Now().UnixNano()
		rsp, err := http.Get(url)
		if err != nil {
			stat.failed += 1
		} else {
			stat.successed += 1
            /*
			bytes, err := ioutil.ReadAll(rsp.Body)
			if err == nil {
				fmt.Printf("response: %s\n", string(bytes))
			}
            */
			rsp.Body.Close()
		}
		end := time.Now().UnixNano()
		elapsedMs := (int64(end) - int64(start)) / int64(time.Millisecond)

		stat.respTimeMsTotal += elapsedMs
		if elapsedMs > stat.respTimeMsMax {
			stat.respTimeMsMax = elapsedMs
		}
	}
}

func createWorkers(args *Args, testData []string, stats []Stat, wc chan<- int) {
	params := make([]Param, args.concurrency)

	for i := 0; i < args.concurrency; i++ {
		params[i].data = testData
		params[i].task = generateTask(args.queryCnt, len(testData))
		params[i].queryCnt = args.queryCnt
		go work(args.serviceAddr, &params[i], &stats[i], wc)
	}
}

func main() {

	c := flag.Int("c", 1, "number of concurrent clients")
	n := flag.Int("n", 1, "number of request per client")
	h := flag.String("h", "http://localhost", "service address")
	d := flag.String("d", "", "path of test data")

	flag.Parse()

	args := &Args{
		concurrency:  *c,
		queryCnt:     *n,
		serviceAddr:  *h,
		testDataPath: *d,
	}

	fmt.Printf("Args: %#v\n", *args)

	data, err := loadData(args.testDataPath)
	if err != nil {
		log.Fatalf("Open %s failed: %v\n", args.testDataPath, err)
	}

	// used for synchronization
	wc := make(chan int, args.concurrency)

	// updated by go worker routines
	stats := make([]Stat, args.concurrency)

	// create worker routines
	createWorkers(args, data, stats, wc)

	// wait for worker finish
	for i := 0; i < args.concurrency; i++ {
		<-wc
	}

	// report statistics
	for i := 0; i < args.concurrency; i++ {
		fmt.Printf("thread %d finished: %#v\n", i, stats[i])
	}
}
