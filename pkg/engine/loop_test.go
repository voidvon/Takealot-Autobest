package engine

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestConcurrentLogDisconnectAndSlowSubscriber(t *testing.T) {
	eng := NewEngine(nil, nil, nil, nil)
	slow := eng.SubscribeLogs() // Deliberately never read: log producers must not block.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for j := 0; j < 2000; j++ {
				eng.Log("concurrent log", "INFO")
			}
		})
		wg.Go(func() {
			for j := 0; j < 2000; j++ {
				ch := eng.SubscribeLogs()
				runtime.Gosched()
				eng.UnsubscribeLogs(ch)
				eng.UnsubscribeLogs(ch) // Duplicate cleanup is harmless.
			}
		})
	}
	wg.Wait()
	eng.UnsubscribeLogs(slow)
	if got := len(eng.GetRecentLogs()); got != 500 {
		t.Fatalf("log retention=%d want 500", got)
	}
}

func TestRepriceLoopSchedulesAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eng := NewEngine(nil, nil, nil, nil)
		if ok, message := eng.StartReprice("shop"); !ok {
			t.Fatal(message)
		}
		defer eng.StopReprice("shop")
		synctest.Wait()
		status := eng.GetStatus("shop")
		if status.NextRunTime == nil || status.CountdownSeconds != 300 {
			t.Fatalf("next cycle not scheduled from completion: %+v", status)
		}
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		if next := eng.GetStatus("shop"); *next.LastRunTime == *status.LastRunTime || next.CountdownSeconds != 300 {
			t.Fatalf("second cycle did not run and reschedule: %+v", next)
		}
		if ok, message := eng.StopReprice("shop"); !ok {
			t.Fatal(message)
		}
		synctest.Wait()
		status = eng.GetStatus("shop")
		if status.IsRunning || status.NextRunTime != nil || status.CountdownSeconds != 0 {
			t.Fatalf("stopped loop still has a scheduled cycle: %+v", status)
		}
		lastRun := *status.LastRunTime
		time.Sleep(5 * time.Minute)
		if *eng.GetStatus("shop").LastRunTime != lastRun {
			t.Fatal("stopped loop started another cycle")
		}
		logs := eng.GetRecentLogs()
		if !strings.Contains(logs[1].Message, "等待下一轮") {
			t.Fatalf("waiting phase was not logged: %+v", logs)
		}
	})
}

func TestAllStatusDoesNotCountDownWhileAStoreIsChecking(t *testing.T) {
	eng := NewEngine(nil, nil, nil, nil)
	checking := eng.getOrCreateWorker("checking")
	checking.isRunning = true
	waiting := eng.getOrCreateWorker("waiting")
	waiting.isRunning = true
	waiting.nextRunTime = time.Now().Add(time.Minute)
	status := eng.GetAllStatus()
	if !status.IsRunning || status.NextRunTime != nil || status.CountdownSeconds != 0 {
		t.Fatalf("a waiting store hid the active cycle: %+v", status)
	}
}
