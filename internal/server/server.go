package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/notify"
)

const (
	defaultBind     = "0.0.0.0:7777"
	readTimeout     = 5 * time.Second
	writeTimeout    = 10 * time.Second
	idleTimeout     = 60 * time.Second
	shutdownTimeout = 5 * time.Second
)

type Options struct {
	Bind    string
	LogPath string
	Version string
	Ready   chan<- struct{}
}

type Server struct {
	cfg        *config.Config
	dispatcher *notify.Dispatcher
	opts       Options
	startedAt  time.Time
	mux        *http.ServeMux

	mu         sync.Mutex
	httpServer *http.Server
	listener   net.Listener
}

func New(cfg *config.Config, dispatcher *notify.Dispatcher, opts Options) *Server {
	if opts.Bind == "" {
		opts.Bind = defaultBind
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if dispatcher == nil {
		dispatcher = notify.NewDispatcher(nil)
	}

	s := &Server{
		cfg:        cfg,
		dispatcher: dispatcher,
		opts:       opts,
		startedAt:  time.Now(),
	}
	s.mux = s.routes()
	return s
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.opts.Bind)
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Handler:      s.mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	s.mu.Lock()
	s.listener = listener
	s.httpServer = httpServer
	s.mu.Unlock()

	if s.opts.Ready != nil {
		close(s.opts.Ready)
	}
	slog.Info("server started", "bind", s.opts.Bind)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			slog.Warn("shutdown error", "err", err)
			return err
		}
		slog.Info("shutdown complete")
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	httpServer := s.httpServer
	s.mu.Unlock()

	if httpServer == nil {
		return nil
	}
	return httpServer.Shutdown(ctx)
}

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /notify", panicRecoveryMiddleware(
		AuthMiddleware(s.cfg.Auth.Key, http.HandlerFunc(s.notifyHandler)),
	))
	mux.Handle("GET /health", panicRecoveryMiddleware(
		http.HandlerFunc(s.healthHandler),
	))
	return mux
}
