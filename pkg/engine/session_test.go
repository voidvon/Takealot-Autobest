package engine

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestVerificationCancelsCalculationAndDoesNotRestartStoppedWorker(t *testing.T) {
	eng := NewEngine(nil, nil, nil, nil)
	worker := eng.getOrCreateWorker("a")
	ctx, cancel := context.WithCancel(context.Background())
	worker.cancelCycle = cancel
	worker.isRunning = true
	eng.SetVerificationRequired("a", true, "waiting")
	if ctx.Err() != context.Canceled {
		t.Fatal("current calculation not canceled")
	}
	if status := eng.GetStatus("a"); !status.VerificationRequired || !status.IsPaused {
		t.Fatalf("pause not visible: %+v", status)
	}
	eng.StopReprice("a")
	eng.SetVerificationRequired("a", false, "ready")
	if status := eng.GetStatus("a"); status.IsRunning || status.VerificationRequired {
		t.Fatalf("verification restarted stopped task: %+v", status)
	}
}
func TestVerificationWakesScheduledCycleButPreservesManualPause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		eng := NewEngine(nil, nil, nil, nil)
		eng.StartReprice("a")
		defer eng.StopReprice("a")
		synctest.Wait()
		original := *eng.GetStatus("a").LastRunTime
		time.Sleep(time.Second)
		eng.SetVerificationRequired("a", true, "waiting")
		synctest.Wait()
		if eng.GetStatus("a").NextRunTime != nil {
			t.Fatal("waiting for verification still has countdown")
		}
		time.Sleep(time.Second)
		eng.SetVerificationRequired("a", false, "ready")
		time.Sleep(300 * time.Millisecond)
		synctest.Wait()
		if next := eng.GetStatus("a"); *next.LastRunTime == original {
			t.Fatal("verification did not start a fresh cycle")
		}
		eng.PauseReprice("a")
		eng.SetVerificationRequired("a", true, "waiting")
		eng.SetVerificationRequired("a", false, "ready")
		synctest.Wait()
		if !eng.GetStatus("a").IsPaused {
			t.Fatal("verification erased user pause")
		}
	})
}
