package main

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/colega/zeropool"
	"github.com/docopt/docopt-go"
	"github.com/xdg-go/pbkdf2"
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

	walletPool = sync.Pool{
		New: func() any {
			return new(Wallet)
		},
	}

	seedPool = zeropool.New(func() []byte {
		return make([]byte, _maxSeedSize)
	})
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

		r := walletPool.Get().(*Wallet)
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
	seed := seedPool.Get()

	for {
		size := 0
		for i := 0; i < _Words; i++ {
			for {
				x, err := rand.Int(rand.Reader, _wordsSize)
				if err != nil {
					continue
				}

				for w := 0; w < len(wordsArr[x.Uint64()]); w++ {
					seed[size+w] = wordsArr[x.Uint64()][w]
				}

				size += len(wordsArr[x.Uint64()])

				if i != _Words-1 {
					seed[size] = ' '
					size += 1
				}

				break
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

		return seed, size, addr.Bounce(false)
	}
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

	walletLength := base64.RawURLEncoding.EncodedLen(36)

	lastTick := started
	lastTotal := 0

	for {
		select {
		case wallet := <-fanout:
			found := false
			for i := 0; i < len(suffixes); i++ {
				if suffixes[i] == strings.ToLower(
					wallet.Addr.String()[walletLength-len(suffixes[i]):],
				) {
					found = true
					break
				}
			}

			if found {
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
				)
			}

			seedPool.Put(wallet.Seed)
			wallet.Addr = nil
			wallet.Seed = nil
			walletPool.Put(wallet)

		case <-ticker.C:
			log.Println(
				"[ticker] since last tick:", total-lastTotal,
				"speed w/m:", float64(total-lastTotal)/time.Since(lastTick).Minutes(),
			)
		}
	}
}
