package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	//"io/ioutil"
	"crypto/tls"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

type Args struct {
	concurrency   int
	queryCnt      int
	serviceAddr   string
	testDataPath  string
	reportDetails bool
	defaults      string
}

type Param struct {
	data     []string
	task     []int
	queryCnt int
}

type Stat struct {
	failed          int
	successed       int
	respTimeTotalMs int64
	respTimeAvgMs   int64
	respTimeMaxMs   int64
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

func work(addr, defaults string, param *Param, stat *Stat, wc chan<- int) {
	defer func() {
		wc <- 1
		if stat.failed > 0 || stat.successed > 0 {
			stat.respTimeAvgMs = stat.respTimeTotalMs / int64(stat.failed+stat.successed)
		}
	}()

    client := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
        },
    }

	for i := 0; i < param.queryCnt; i++ {

		url := addr
		vars := strings.TrimSpace(param.data[param.task[i]])
		defaults := strings.TrimSpace(defaults)
		if defaults != "" {
			url += "?" + defaults
			if vars != "" {
				url += "&" + vars
			}
		} else if vars != "" {
			url += "?" + vars
		}

		start := time.Now().UnixNano()
		rsp, err := client.Get(url)
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

		stat.respTimeTotalMs += elapsedMs
		if elapsedMs > stat.respTimeMaxMs {
			stat.respTimeMaxMs = elapsedMs
		}
	}
}

func createWorkers(args *Args, testData []string, stats []Stat, wc chan<- int) {
	params := make([]Param, args.concurrency)

	for i := 0; i < args.concurrency; i++ {
		params[i].data = testData
		params[i].task = generateTask(args.queryCnt, len(testData))
		params[i].queryCnt = args.queryCnt
		go work(args.serviceAddr, args.defaults, &params[i], &stats[i], wc)
	}
}

func main() {

	c := flag.Int("c", 1, "number of concurrent clients")
	n := flag.Int("n", 1, "number of request per client")
	h := flag.String("h", "http://localhost", "service address")
	f := flag.String("f", "", "path of test data")
	v := flag.Bool("v", false, "report details of each client when finished")
	d := flag.String("d", "", "defalut value")

	flag.Parse()

	args := &Args{
		concurrency:   *c,
		queryCnt:      *n,
		serviceAddr:   *h,
		testDataPath:  *f,
		reportDetails: *v,
		defaults:      *d,
	}

	log.Printf("Args: %#v\n", *args)

	if args.concurrency <= 0 {
		fmt.Printf("concurrency must be greater than 0\n")
		return
	}

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
	var respTimeAvgMs int64 = 0
	var respTimeMaxMs int64 = 0
	var failed int64 = 0
	var qps int64 = 0

	for i := 0; i < args.concurrency; i++ {
		if args.reportDetails {
			log.Printf("thread %d: %#v\n", i, stats[i])
		}
		failed += int64(stats[i].failed)
		respTimeAvgMs += stats[i].respTimeAvgMs
		if stats[i].respTimeMaxMs > respTimeMaxMs {
			respTimeMaxMs = stats[i].respTimeMaxMs
		}
		qps += int64(args.queryCnt*1000) / int64(stats[i].respTimeTotalMs)
	}

	respTimeAvgMs /= int64(args.concurrency)
	log.Printf("Total request: %d, failed: %d, avg response time: %d ms, max response time: %d ms, QPS: %d\n",
		args.concurrency*args.queryCnt, failed, respTimeAvgMs, respTimeMaxMs, qps)
}
