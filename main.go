package main

import (
	"crypto/ed25519"
	"crypto/hmac"

	//"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"

	fastrand "github.com/valyala/fastrand"

	"github.com/docopt/docopt-go"
	//"github.com/xdg-go/pbkdf2"

	//"github.com/xdg-go/pbkdf2"
	//"golang.org/x/crypto/pbkdf2"

	pbkdf2 "github.com/ctz/go-fastpbkdf2"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/ton/wallet"
)

const (
	_globalConfigURL = "https://ton.org/global.config.json"
)

var (
	usage = `walleter 1.0

Usage:
  walleter [options] -t <threads> <suffix>...
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
	Addr     *address.Address
	Seed     []byte
	SeedSize int
}

type Arguments struct {
	Threads   int
	Addresses string
	Seeds     string
	Suffixes  []string `docopt:"<suffix>"`
}

var (
	walletVersion = wallet.V4R2

	//walletPool = sync.Pool{
	//    New: func() any {
	//        return new(Wallet)
	//    },
	//}

	//seedPool = zeropool.New(func() []byte {
	//    return make([]byte, _maxSeedSize)
	//})
)

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

	for i, suffix := range args.Suffixes {
		args.Suffixes[i] = strings.ToLower(suffix)
	}

	if os.Getenv("PPROF") != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.HandleFunc("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
		mux.HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
		mux.HandleFunc("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
		mux.HandleFunc("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
		mux.HandleFunc("/debug/pprof/mutex", pprof.Handler("mutex").ServeHTTP)

		go func() {
			if err := http.ListenAndServe(os.Getenv("PPROF"), mux); err != nil {
				panic(err)
			}
		}()
	}

	//client := liteclient.NewConnectionPool()

	//err = client.AddConnectionsFromConfigUrl(
	//    context.Background(),
	//    _globalConfigURL,
	//)
	//if err != nil {
	//    log.Fatalf("initialize liteclient: %v", err)
	//}

	//api := ton.NewAPIClient(client).WithRetry()

	fanout := make(chan *Wallet, 10000)

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

	wg := sync.WaitGroup{}

	wg.Add(args.Threads)
	for i := 0; i < args.Threads; i++ {
		go func() {
			defer wg.Done()

			generate(fanout)
		}()
	}

	// probably should add wg here too
	go filter(args.Suffixes, fanout, addrFile, seedFile)

	wg.Wait()
}

func generate(fanout chan *Wallet) {
	for {
		seed, size, addr := NewWallet()

		//r := walletPool.Get().(*Wallet)
		r := &Wallet{}
		r.Seed = seed
		r.SeedSize = size
		r.Addr = addr

		fanout <- r
	}
}

const (
	_Iterations   = 100000
	_Salt         = "TON default seed"
	_BasicSalt    = "TON seed version"
	_PasswordSalt = "TON fast seed version"
	_Words        = 24
)

func NewWallet() ([]byte, int, *address.Address) {
	//seed := seedPool.Get()
	seed := make([]byte, _maxSeedSize)

	for {
		size := 0
		for i := 0; i < _Words; i++ {
			x := fastrand.Uint32n(_wordsSizeUint32)

			for w := 0; w < len(wordsArr[x]); w++ {
				seed[size+w] = wordsArr[x][w]
			}

			size += len(wordsArr[x])

			if i != _Words-1 {
				seed[size] = ' '
				size += 1
			}
		}

		mac := hmac.New(sha512.New, seed[:size])
		hash := mac.Sum(nil)

		p := pbkdf2.Key(hash, []byte(_BasicSalt), _Iterations/256, 1, sha512.New)
		if p[0] != 0 {
			continue
		}

		k := pbkdf2.Key(hash, []byte(_Salt), _Iterations, 32, sha512.New)
		key := ed25519.NewKeyFromSeed(k)

		addr, err := wallet.AddressFromPubKey(key.Public().(ed25519.PublicKey), walletVersion, wallet.DefaultSubwallet)
		if err != nil {
			panic(err)
		}

		addr.SetBounce(false)

		return seed, size, addr
	}
}

var walletLength = base64.RawURLEncoding.EncodedLen(36)

func match(addr *address.Address, suffixes []string) bool {
	for i := 0; i < len(suffixes); i++ {
		if suffixes[i] == strings.ToLower(
			addr.String()[walletLength-len(suffixes[i]):],
		) {
			return true
		}
	}

	return false
}

func filter(
	suffixes []string,
	fanout chan *Wallet,
	addrFile *os.File,
	seedFile *os.File,
) {
	total := 0
	started := time.Now()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	lastTick := started
	lastTotal := 0

	for {
		select {
		case wallet := <-fanout:
			if match(wallet.Addr, suffixes) {
				_, err := seedFile.WriteString(
					wallet.Addr.String() + " " + string(wallet.Seed[:wallet.SeedSize]) + "\n",
				)
				if err != nil {
					log.Fatalln("write seed file:", err.Error())
					return
				}

				_, err = addrFile.WriteString(wallet.Addr.String() + "\n")
				if err != nil {
					log.Fatalln("write addr file:", err.Error())
					return
				}

				log.Println(wallet.Addr.String())
			}

			total++
			if total%100 == 0 {
				log.Println(
					"total:",
					total,
					"time:",
					time.Since(started),
					"avg w/s:",
					float64(total)/time.Since(started).Seconds(),
					"avg w/m:",
					float64(total)/time.Since(started).Minutes(),
					"queue load:", float64(len(fanout))/float64(cap(fanout))*100.0,
				)
			}

			//seedPool.Put(wallet.Seed)
			//wallet.Addr = nil
			//wallet.Seed = nil
			//walletPool.Put(wallet)

		case <-ticker.C:
			log.Println(
				"[ticker] since last tick:", total-lastTotal,
				"speed w/m:", float64(total-lastTotal)/time.Since(lastTick).Minutes(),
			)
		}
	}
}
