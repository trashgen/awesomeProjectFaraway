package server

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"

	"awesomeProjectFaraway/internal/pkg/config"
	"awesomeProjectFaraway/internal/pkg/pow"
	"awesomeProjectFaraway/internal/pkg/protocol"
)

// Quotes - const array of quotes to respond on client's request
var Quotes = []string{
	"All saints who remember to keep and do these sayings, " +
		"walking in obedience to the commandments, " +
		"shall receive health in their navel and marrow to their bones",

	"And shall find wisdom and great treasures of knowledge, even hidden treasures",

	"And shall run and not be weary, and shall walk and not faint",

	"And I, the Lord, give unto them a promise, " +
		"that the destroying angel shall pass by them, " +
		"as the children of Israel, and not slay them",
}

var ErrQuit = errors.New("client requests to close connection")

// Clock  - interface for easier mock time.Now in tests
type Clock interface {
	Now() time.Time
}

// Cache - interface for add, delete and check existence of rand values for hashcash
type Cache interface {
	// Add - add rand value with expiration (in seconds) to cache
	Add(int, int64) error
	// Get - check existence of int key in cache
	Get(int) (bool, error)
	// Delete - delete key from cache
	Delete(int)
}

// Run - main function, launches server to listen on given address and handle new connections
func Run(ctx context.Context, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	// Close the listener when the application closes.
	defer listener.Close()
	fmt.Println("listening", listener.Addr())
	for {
		// Listen for an incoming connection.
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("error accept connection: %w", err)
		}
		// Handle connections in a new goroutine.
		go handleConnection(ctx, conn)
	}
}

func handleConnection(ctx context.Context, conn net.Conn) {
	fmt.Println("new client:", conn.RemoteAddr())
	defer conn.Close()

	reader := bufio.NewReader(conn)

	for {
		req, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("err read connection:", err)
			return
		}
		msg, err := ProcessRequest(ctx, req, conn.RemoteAddr().String())
		if err != nil {
			fmt.Println("err process request:", err)
			return
		}
		if msg != nil {
			err := sendMsg(*msg, conn)
			if err != nil {
				fmt.Println("err send message:", err)
			}
		}
	}
}

// ProcessRequest - process request from client
// returns not-nil pointer to Message if needed to send it back to client
func ProcessRequest(ctx context.Context, msgStr string, clientInfo string) (*protocol.Message, error) {
	msg, err := protocol.ParseMessage(msgStr)
	if err != nil {
		return nil, err
	}
	// switch by header of msg
	switch msg.Header {
	case protocol.Quit:
		return nil, ErrQuit
	case protocol.RequestChallenge:
		fmt.Printf("client %s requests challenge\n", clientInfo)
		// create new challenge for client
		conf := ctx.Value("config").(*config.Config)
		clock := ctx.Value("clock").(Clock)
		cache := ctx.Value("cache").(Cache)
		date := clock.Now()

		// add new created rand value to cache to check it later on RequestResource stage
		// with duration in seconds
		randValue := rand.Intn(100000)
		err := cache.Add(randValue, conf.HashcashDuration)
		if err != nil {
			return nil, fmt.Errorf("err add rand to cache: %w", err)
		}

		hashcash := pow.HashcashData{
			Version:    1,
			ZerosCount: conf.HashcashZerosCount,
			Date:       date.Unix(),
			Resource:   clientInfo,
			Rand:       base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", randValue))),
			Counter:    0,
		}
		hashcashMarshaled, err := json.Marshal(hashcash)
		if err != nil {
			return nil, fmt.Errorf("err marshal hashcash: %v", err)
		}
		msg := protocol.Message{
			Header:  protocol.ResponseChallenge,
			Payload: string(hashcashMarshaled),
		}
		return &msg, nil
	case protocol.RequestResource:
		fmt.Printf("client %s requests resource with payload %s\n", clientInfo, msg.Payload)
		// parse client's solution
		var hashcash pow.HashcashData
		err := json.Unmarshal([]byte(msg.Payload), &hashcash)
		if err != nil {
			return nil, fmt.Errorf("err unmarshal hashcash: %w", err)
		}
		// validate hashcash params
		if hashcash.Resource != clientInfo {
			return nil, fmt.Errorf("invalid hashcash resource")
		}
		conf := ctx.Value("config").(*config.Config)
		clock := ctx.Value("clock").(Clock)
		cache := ctx.Value("cache").(Cache)

		// decoding rand from base64 field in received client's hashcash
		randValueBytes, err := base64.StdEncoding.DecodeString(hashcash.Rand)
		if err != nil {
			return nil, fmt.Errorf("err decode rand: %w", err)
		}
		randValue, err := strconv.Atoi(string(randValueBytes))
		if err != nil {
			return nil, fmt.Errorf("err decode rand: %w", err)
		}

		// if rand exists in cache, it means, that hashcash is valid and really challenged by this server in past
		exists, err := cache.Get(randValue)
		if err != nil {
			return nil, fmt.Errorf("err get rand from cache: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("challenge expired or not sent")
		}

		// sent solution should not be outdated
		if clock.Now().Unix()-hashcash.Date > conf.HashcashDuration {
			return nil, fmt.Errorf("challenge expired")
		}
		//to prevent indefinite computing on server if client sent hashcash with 0 counter
		maxIter := hashcash.Counter
		if maxIter == 0 {
			maxIter = 1
		}
		_, err = hashcash.ComputeHashcash(maxIter)
		if err != nil {
			return nil, fmt.Errorf("invalid hashcash")
		}
		//get random quote
		fmt.Printf("client %s succesfully computed hashcash %s\n", clientInfo, msg.Payload)
		msg := protocol.Message{
			Header:  protocol.ResponseResource,
			Payload: Quotes[rand.Intn(4)],
		}
		// delete rand from cache to prevent duplicated request with same hashcash value
		cache.Delete(randValue)
		return &msg, nil
	default:
		return nil, fmt.Errorf("unknown header")
	}
}

// sendMsg - send protocol message to connection
func sendMsg(msg protocol.Message, conn net.Conn) error {
	msgStr := fmt.Sprintf("%s\n", msg.Stringify())
	_, err := conn.Write([]byte(msgStr))
	return err
}
