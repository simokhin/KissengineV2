package main

import (
	"fmt"
	"io"
	"kissengine-bitboard/engine"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer collects the engine's output; the search goroutine writes to it
// while the test reads.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// session runs the UCI loop on a pipe so a test can send commands over time,
// like a GUI does.
type session struct {
	t   *testing.T
	in  *io.PipeWriter
	out *syncBuffer
}

func newSession(t *testing.T) *session {
	pr, pw := io.Pipe()
	s := &session{t: t, in: pw, out: &syncBuffer{}}

	done := make(chan struct{})
	go func() {
		run(pr, s.out)
		close(done)
	}()
	t.Cleanup(func() {
		pw.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("run did not return after stdin closed")
		}
	})

	s.send("setoption name Hash value 16")
	return s
}

func (s *session) send(cmd string) {
	fmt.Fprintln(s.in, cmd)
}

// waitFor polls the output until it contains substr.
func (s *session) waitFor(substr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(s.out.String(), substr) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestGoInfiniteRunsUntilStop(t *testing.T) {
	s := newSession(t)
	s.send("position startpos")
	s.send("go infinite")

	if !s.waitFor("info depth 3 ", 5*time.Second) {
		t.Fatalf("no progress reports:\n%s", s.out.String())
	}

	// Well past the old flat one second, still no move.
	time.Sleep(1200 * time.Millisecond)
	if strings.Contains(s.out.String(), "bestmove") {
		t.Fatalf("infinite search finished by itself:\n%s", s.out.String())
	}

	start := time.Now()
	s.send("stop")
	if !s.waitFor("bestmove ", 2*time.Second) {
		t.Fatalf("no bestmove after stop:\n%s", s.out.String())
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("stop took %v to produce a move", d)
	}
}

func TestIsreadyAnsweredDuringSearch(t *testing.T) {
	s := newSession(t)
	s.send("position startpos")
	s.send("go infinite")
	time.Sleep(100 * time.Millisecond)

	s.send("isready")
	if !s.waitFor("readyok", 500*time.Millisecond) {
		t.Fatal("isready was not answered while searching")
	}
	s.send("stop")
	s.waitFor("bestmove ", 2*time.Second)
}

func TestGoMovetime(t *testing.T) {
	s := newSession(t)
	s.send("position startpos")

	start := time.Now()
	s.send("go movetime 300")
	if !s.waitFor("bestmove ", 2*time.Second) {
		t.Fatalf("no bestmove:\n%s", s.out.String())
	}

	if d := time.Since(start); d < 250*time.Millisecond || d > 900*time.Millisecond {
		t.Errorf("movetime 300 took %v", d)
	}
}

func TestGoDepthReportsEachDepthAndPV(t *testing.T) {
	s := newSession(t)
	s.send("position startpos")
	s.send("go depth 4")

	if !s.waitFor("bestmove ", 5*time.Second) {
		t.Fatalf("no bestmove:\n%s", s.out.String())
	}

	lines := strings.Split(strings.TrimSpace(s.out.String()), "\n")
	var infos []string
	for _, l := range lines {
		if strings.HasPrefix(l, "info depth ") {
			infos = append(infos, l)
		}
	}
	if len(infos) != 4 {
		t.Fatalf("got %d info lines, want 4:\n%s", len(infos), s.out.String())
	}

	last := strings.Fields(infos[3])
	pvAt := -1
	for i, f := range last {
		if f == "pv" {
			pvAt = i
		}
	}
	if pvAt < 0 || pvAt+1 >= len(last) {
		t.Fatalf("no pv in %q", infos[3])
	}
	for _, key := range []string{"nodes", "nps", "time", "score"} {
		if !strings.Contains(infos[3], " "+key+" ") {
			t.Errorf("info line lacks %q: %q", key, infos[3])
		}
	}

	bestmove := strings.Fields(lines[len(lines)-1])
	if bestmove[0] != "bestmove" || bestmove[1] != last[pvAt+1] {
		t.Errorf("bestmove %v does not match the pv head %s", bestmove, last[pvAt+1])
	}
}

func TestBestmove0000WhenMated(t *testing.T) {
	s := newSession(t)
	s.send("position fen rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3")
	s.send("go depth 3")

	if !s.waitFor("bestmove 0000", 2*time.Second) {
		t.Fatalf("want bestmove 0000:\n%s", s.out.String())
	}
}

// Piped input without "quit": a finite search still prints its move before
// run returns, an infinite one is cancelled instead of hanging.
func TestEOFHandling(t *testing.T) {
	var out syncBuffer
	run(strings.NewReader("setoption name Hash value 16\nposition startpos\ngo depth 3\n"), &out)
	if !strings.Contains(out.String(), "bestmove ") {
		t.Errorf("finite search lost its bestmove at EOF:\n%s", out.String())
	}

	out = syncBuffer{}
	done := make(chan struct{})
	go func() {
		run(strings.NewReader("setoption name Hash value 16\ngo infinite\n"), &out)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("run hung on an infinite search at EOF")
	}
}

func TestUciAdvertisesHash(t *testing.T) {
	var out syncBuffer
	run(strings.NewReader("uci\nquit\n"), &out)

	if !strings.Contains(out.String(), "option name Hash type spin") || !strings.Contains(out.String(), "uciok") {
		t.Errorf("unexpected uci reply:\n%s", out.String())
	}
}

func TestParseGoAndTimeLimit(t *testing.T) {
	tests := []struct {
		cmd       string
		side      engine.Color
		wantTimed bool
		wantMs    int
		wantDepth int
		wantInf   bool
	}{
		{"go infinite", engine.White, false, 0, 0, true},
		{"go", engine.White, false, 0, 0, false},
		{"go depth 7", engine.White, false, 0, 7, false},
		{"go movetime 500", engine.White, true, 500, 0, false},
		{"go wtime 60000 btime 30000 winc 1000 binc 500", engine.White, true, 3000, 0, false},
		{"go wtime 60000 btime 30000 winc 1000 binc 500", engine.Black, true, 1500, 0, false},
		{"go wtime 60000 btime 60000 movestogo 10", engine.White, true, 6000, 0, false},
		{"go wtime 60000 btime 60000 movestogo 1", engine.White, true, 30000, 0, false},
		{"go wtime 60000 btime 60000 movestogo 100", engine.White, true, 2000, 0, false},
		{"go wtime 0 btime 0", engine.White, true, 50, 0, false},
		{"go infinite wtime 60000 btime 60000", engine.White, false, 0, 0, true},
		{"go searchmoves e2e4 d2d4 depth 3", engine.White, false, 0, 3, false},
	}

	for _, tt := range tests {
		l := parseGo(strings.Fields(tt.cmd))
		d, timed := l.timeLimit(tt.side)

		if timed != tt.wantTimed || int(d/time.Millisecond) != tt.wantMs || l.depth != tt.wantDepth || l.infinite != tt.wantInf {
			t.Errorf("%q (side %d): timed=%v %dms depth=%d infinite=%v, want timed=%v %dms depth=%d infinite=%v",
				tt.cmd, tt.side, timed, d/time.Millisecond, l.depth, l.infinite,
				tt.wantTimed, tt.wantMs, tt.wantDepth, tt.wantInf)
		}
	}
}

func TestHardLimit(t *testing.T) {
	tests := []struct {
		cmd    string
		side   engine.Color
		wantMs int
	}{
		{"go movetime 500", engine.White, 500},                                // exact: no room to grow
		{"go wtime 60000 btime 30000 winc 1000 binc 500", engine.White, 9000}, // 3 x the 3000 ms budget
		{"go wtime 60000 btime 30000 winc 1000 binc 500", engine.Black, 4500}, // 3 x 1500, a quarter of 30 s + 0.5 s is 8000
		{"go wtime 1000 btime 1000 winc 100 binc 100", engine.White, 350},     // a quarter of the clock plus the increment caps 3 x 133
		{"go wtime 60000 btime 60000 movestogo 1", engine.White, 30000},       // the budget itself is already above a quarter of the clock
		{"go wtime 0 btime 0", engine.White, 50},                              // the floor budget, nothing to spare
		{"go infinite", engine.White, 0},
	}

	for _, tt := range tests {
		l := parseGo(strings.Fields(tt.cmd))
		budget, _ := l.timeLimit(tt.side)

		if got := l.hardLimit(tt.side, budget); int(got/time.Millisecond) != tt.wantMs {
			t.Errorf("%q (side %d): hard limit %v, want %dms (budget %v)", tt.cmd, tt.side, got, tt.wantMs, budget)
		}
	}
}

func TestFormatScore(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{150, "score cp 150"},
		{-20, "score cp -20"},
		{engine.MateValue - 1, "score mate 1"}, // we mate on our next move
		{engine.MateValue - 3, "score mate 2"}, // mate on our second move
		{engine.MateValue - 5, "score mate 3"},
		{-(engine.MateValue - 2), "score mate -1"}, // we are mated on their next move
		{-(engine.MateValue - 4), "score mate -2"},
	}

	for _, tt := range tests {
		if got := formatScore(tt.score); got != tt.want {
			t.Errorf("formatScore(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}
