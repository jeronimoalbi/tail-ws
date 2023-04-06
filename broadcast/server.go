package broadcast

import (
	"bufio"
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"

	"github.com/jeronimoalbi/tail-ws/watch"
)

var (
	// DefaultAddr defines the default Websocket server address.
	DefaultAddr = "127.0.0.1:8080"

	maxMessageSize int64 = 1024
	pingPeriod           = (pongWait * 9) / 10
	pongWait             = 60 * time.Second
	writeWait            = 8 * time.Second
)

// Option configures transaction broadcast servers.
type Option func(*Server)

// Address sets the server address.
func Address(addr string) Option {
	return func(s *Server) {
		s.addr = addr
	}
}

// Origin sets the allowed origin for incoming requests.
func Origin(origin string) Option {
	return func(s *Server) {
		s.origin = origin
	}
}

// Secure enables secure Websockets (WSS).
func Secure(certFile, keyFile string) Option {
	return func(s *Server) {
		s.certFile = certFile
		s.keyFile = keyFile
	}
}

// NewServer creates a new transactions broadcast server.
func NewServer(options ...Option) *Server {
	s := Server{
		addr:        DefaultAddr,
		connections: NewConnections(),
	}

	for _, apply := range options {
		apply(&s)
	}

	s.upgrader.CheckOrigin = func(r *http.Request) bool {
		if s.origin != "" {
			return s.origin == r.Header.Get("Origin")
		}
		return true
	}

	return &s
}

// Server handles Websocket connections and broadcasts new transactions.
// It watches the transactions head file and when new transactions are indexed
// it pushes the new entries to the connected clients.
type Server struct {
	addr, origin, certFile, keyFile string
	reader                          watch.Reader
	connections                     *Connections
	upgrader                        websocket.Upgrader
}

// HandleWS is an HTTP handler that upgrades incoming connections to WS or WSS.
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	// TODO: Add authentication support
	log.Printf("connection stablished with %s", r.RemoteAddr)

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already returns the error to the client on failure
		log.Printf("connection from %s failed: %v", r.RemoteAddr, err)
		return
	}

	ws.SetReadLimit(maxMessageSize)

	// Prepare keep alive protocol for the new connection
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Launch a gopher to keep connection alive
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait))
				if err != nil {
					log.Printf("error sending ping: %v", err)
					ws.Close()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Make sure to cleanup connection when closed
	ws.SetCloseHandler(func(int, string) error {
		log.Printf("closing connextion %s", ws.RemoteAddr())
		cancel()
		return s.connections.Delete(ws)
	})

	s.connections.Add(ws)
}

// Start starts a new HTTP server to listen for incoming WS or WSS connections.
func (s *Server) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	server := &http.Server{
		Addr:    s.addr,
		Handler: http.HandlerFunc(s.HandleWS),
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	g.Go(func() error {
		<-ctx.Done()
		s.connections.Close()
		return server.Close()
	})

	g.Go(func() error {
		var err error
		if s.certFile != "" && s.keyFile != "" {
			log.Printf("listening for connections -> wss://%s", s.addr)
			err = server.ListenAndServeTLS(s.certFile, s.keyFile)
		} else {
			log.Printf("listening for connections -> ws://%s", s.addr)
			err = server.ListenAndServe()
		}

		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})

	return g.Wait()
}

// Watch starts watching a transaction head file and broadcasts
// the newly indexed transactions to all connected peers.
func (s *Server) Watch(ctx context.Context, name string) error {
	r := watch.NewReader(watch.SeekEnd())
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			s.broadcast(scanner.Bytes())
		}

		return scanner.Err()
	})

	g.Go(func() error {
		defer r.Close()

		for {
			// Keep watching when the file is ovewritten
			if err := r.Watch(ctx, name); err != watch.ErrFileOverwritten {
				return err
			}
		}
	})

	return g.Wait()
}

func (s Server) broadcast(tx []byte) {
	s.connections.Iter(func(ws *websocket.Conn) bool {
		go func() {
			ws.SetWriteDeadline(time.Now().Add(writeWait))

			if err := ws.WriteMessage(websocket.BinaryMessage, tx); err != nil {
				log.Printf("tx broadcast failed: %v", err)
				ws.Close()
			}
		}()

		return true
	})
}
