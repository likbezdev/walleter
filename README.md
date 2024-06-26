# Walleter

Walleter is a high-performance Golang tool for generating TON wallets in bulk. It utilizes multiple
threads to maximize throughput and can generate thousands of wallets per second.

## Installation

To install `walleter`, make sure you have Go installed on your system. Then, run the following command:

```
go install github.com/likbezdev/walleter@latest
```

## Usage

```
walleter 1.0

Usage:
  walleter [options] -t <threads> <suffix>...
  walleter -h | --help
  walleter --version

Options:
  -t --threads <n>       Number of threads.
  -a --addresses <path>  Path to file to save addresses. [default: addresses.txt]
  -s --seeds <path>      Path to file to save seeds. [default: seeds.txt]
  -h --help              Show this screen.
  --version              Show version.
```

## About

Walleter is designed to generate TON wallets efficiently. It leverages Golang's concurrency features
to spawn multiple worker goroutines, each generating wallets independently. This parallel processing
approach allows Walleter to achieve impressive generation speeds.

Under the hood, Walleter uses the `tonutils-go` library to interact with the TON blockchain. It
retrieves the global configuration from the official TON endpoint and establishes a connection pool
using the `liteclient` package. This ensures reliable and efficient communication with the TON
network.

## Performance

Walleter's performance is attributed to several key factors:

1. **Concurrency**: By utilizing multiple worker goroutines, Walleter can generate wallets
   concurrently, making full use of available system resources. Each goroutine independently
   generates wallets and sends them to an output channel for further processing.

2. **Buffered Channel**: Walleter uses a buffered channel to collect the generated wallets from the
   worker goroutines. The buffer size is set to 1000, allowing the workers to continue generating
   wallets without blocking, even if the output handling goroutine is temporarily busy.

3. **Efficient Output Handling**: The output handling goroutine receives wallets from the buffered
   channel and writes them to the specified output files. By dedicating a separate goroutine to this
   task, Walleter ensures that I/O operations don't slow down the wallet generation process.

By combining these factors, Walleter achieves remarkable generation speeds, making it suitable for
