package minirehs

import "runtime"

func runGatedWorker(task gatedWorkerTask) {
	task.scratch.clearVerifierText()
	result := gatedResult{out: task.out}
	for i, gate := range task.tasks {
		hit, err := task.db.existsNoReport(int(gate.idx), task.data, int(gate.winLo), task.scratch)
		result.completed = i + 1
		if err != nil {
			result.err = err
			break
		}
		if hit {
			task.out[i] = 1
		} else {
			task.out[i] = 0
		}
	}
	task.scratch.clearVerifierText()
	task.result <- result
}

func runAnchoredWorker(task anchoredWorkerTask) {
	task.result <- task.db.scanAnchoredNoReport(task.data, task.anchorBatch, task.biBatch,
		task.owner, task.scratch, task.anchorOut, task.biOut)
}

func (d *mvsDB) dispatchGated(sc *scratch, task gatedWorkerTask) {
	// The result channels are buffered by one. Inline execution fills exactly
	// the original message; errors and callbacks are still replayed later.
	if len(task.data) < 512 && d.inlineSmallWorkers && runtime.GOMAXPROCS(0) >= 4 {
		runGatedWorker(task)
		return
	}
	sc.startWorkers()
	sc.gatedTasks <- task
}

func (d *mvsDB) dispatchAnchored(sc *scratch, task anchoredWorkerTask) {
	if len(task.data) < 512 && d.inlineSmallWorkers && runtime.GOMAXPROCS(0) >= 4 {
		runAnchoredWorker(task)
		return
	}
	sc.startWorkers()
	sc.anchoredTasks <- task
}
