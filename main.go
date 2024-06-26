package main

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"
)

const (
	_globalConfigURL = "https://ton.org/global.config.json"
)

var (
	usage = `walleter 1.0

Usage:
  walleter [options] -t <threads>
  walleter -h | --help
  walleter --version

Options:
  -t --threads <n>     Number of threads.
  -a --addresses <path>  Path to file to save addresses. [default: addresses.txt]
  -s --seeds <path>      Path to file to save seeds. [default: seeds.txt]
  -h --help              Show this screen.
  --version              Show version.
`
)

type Wallet struct {
	Address string
	Seed    []string
}

type Arguments struct {
	Threads   int
	Addresses string
	Seeds     string
}

func main() {
	var args Arguments

	opts, err := docopt.ParseDoc(usage)
	if err != nil {
		log.Fatalf("parse doc: %v", err)
	}

	err = opts.Bind(&args)
	if err != nil {
		log.Fatalf("parse args: %v", err)
	}

	client := liteclient.NewConnectionPool()

	err = client.AddConnectionsFromConfigUrl(
		context.Background(),
		_globalConfigURL,
	)
	if err != nil {
		log.Fatalf("initialize liteclient: %v", err)
	}

	api := ton.NewAPIClient(client).WithRetry()

	fanout := make(chan *Wallet, 1000)

	addrFile, err := os.OpenFile(
		args.Addresses,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		log.Fatalf("open addresses file: %v", err)
	}
	defer addrFile.Close()

	seedFile, err := os.OpenFile(
		args.Seeds,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		log.Fatalf("open seeds file: %v", err)
	}
	defer seedFile.Close()

	go handleOutput(fanout, addrFile, seedFile)

	wg := sync.WaitGroup{}
	wg.Add(args.Threads)
	for i := 0; i < args.Threads; i++ {
		go func() {
			defer wg.Done()

			for {
				generate(api, fanout)
			}
		}()
	}

	wg.Wait()
}

func generate(api ton.APIClientWrapped, fanout chan *Wallet) {
	seed := wallet.NewSeed()

	w, err := wallet.FromSeed(api, seed, wallet.V4R2)
	if err != nil {
		log.Fatalln("FromSeed err:", err.Error())
		return
	}

	result := &Wallet{
		Address: w.WalletAddress().String(),
		Seed:    seed,
	}

	fanout <- result
}

func handleOutput(fanout chan *Wallet, addrFile, seedFile *os.File) {
	total := 0
	started := time.Now()
	for wallet := range fanout {
		_, err := seedFile.WriteString(
			wallet.Address + " " + strings.Join(wallet.Seed, " ") + "\n",
		)
		if err != nil {
			log.Fatalln("write seed file:", err.Error())
			return
		}

		_, err = addrFile.WriteString(wallet.Address + "\n")
		if err != nil {
			log.Fatalln("write addr file:", err.Error())
			return
		}

		total++
		if total%1000 == 0 {
			log.Println(
				"total:",
				total,
				"time:",
				time.Since(started),
				"avg w/s:",
				float64(total)/time.Since(started).Seconds(),
			)
		}
	}
}
