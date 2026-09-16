package gitstate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestPushCanExceedOldTimeouts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture()
		f.live = ""
		start := time.Now()
		c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
			if name == "git" && args[0] == "push" {
				time.Sleep(2 * time.Minute)
			} else if name == "gh" || name == "git" && args[0] == "ls-remote" {
				time.Sleep(15 * time.Second)
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return f.runner(ctx, dir, name, args...)
		}}
		if err := c.CheckPush(context.Background(), "/repo", []string{"origin", "topic"}); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed <= 50*time.Second || elapsed >= PushTimeout {
			t.Fatalf("unexpected test duration: %v", elapsed)
		}
		if c.commandTimeout != 0 {
			t.Fatal("push modified the caller's client")
		}
	})
}

func TestPushTotalBudgetStillApplies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture()
		start := time.Now()
		c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
			if name == "git" && args[0] == "push" {
				<-ctx.Done()
				return "", ctx.Err()
			}
			return f.runner(ctx, dir, name, args...)
		}}
		err := c.CheckPush(context.Background(), "/repo", nil)
		if err == nil || !strings.Contains(err.Error(), "タイムアウト") {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed != PushTimeout {
			t.Fatalf("elapsed=%v, want %v", elapsed, PushTimeout)
		}
	})
}

func TestPushPreservesShorterCallerDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		start := time.Now()
		c := Client{Runner: func(ctx context.Context, dir, name string, args ...string) (string, error) {
			if name == "git" && args[0] == "push" {
				<-ctx.Done()
				return "", ctx.Err()
			}
			return f.runner(ctx, dir, name, args...)
		}}
		if err := c.CheckPush(ctx, "/repo", nil); err == nil {
			t.Fatal("caller deadline ignored")
		}
		if time.Since(start) != 3*time.Second {
			t.Fatal("caller deadline extended")
		}
	})
}

func TestNonPushCommandBudgetIsUnchanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		c := Client{Runner: func(ctx context.Context, _, _ string, _ ...string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}}
		_, err := c.git(context.Background(), "/repo", "status")
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 12*time.Second {
			t.Fatal(err, time.Since(start))
		}
	})
}
