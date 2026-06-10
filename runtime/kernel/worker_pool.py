"""AI Workstation — Shared Worker Pool (v0.4.0)

Persistent thread pool for controlled task scheduling.
Replaces per-request ThreadPoolExecutor with reusable workers.

Properties:
  - N persistent worker threads (configurable)
  - Each worker pulls from RequestQueue, runs pipeline, returns result
  - Per-worker unique ID for observability
  - Graceful shutdown with drain
  - Worker health monitoring
"""

from __future__ import annotations

import threading
import traceback
from datetime import datetime, timezone
from queue import Full as ResultQueueFull


class WorkerPool:
    """Persistent worker pool for parallel request processing.

    Workers pull from a shared RequestQueue and execute the pipeline
    on each request. Results are placed in a bounded result queue.

    Usage:
        pool = WorkerPool(num_workers=4, queue=request_queue)
        pool.start()
        # ... requests are processed ...
        pool.shutdown()
    """

    def __init__(self, num_workers, request_queue, result_queue, max_results=256):
        self._num_workers = num_workers
        self._request_queue = request_queue
        self._result_queue = result_queue
        self._max_results = max_results
        self._workers = []
        self._running = False
        self._lock = threading.Lock()
        self._shutdown_event = threading.Event()

    def start(self, pipeline_process_fn, services):
        """Start all worker threads.

        Args:
            pipeline_process_fn: callable(ctx) → finalizes and returns ctx
            services: PipelineServices instance
        """
        with self._lock:
            if self._running:
                return
            self._running = True
            self._shutdown_event.clear()

            for i in range(self._num_workers):
                worker_id = f"wrk_{i:03d}"
                worker = threading.Thread(
                    target=self._worker_loop,
                    args=(worker_id, pipeline_process_fn, services),
                    name=f"gateway-{worker_id}",
                    daemon=True,
                )
                self._workers.append(worker)
                worker.start()

    def _worker_loop(self, worker_id, process_fn, services):
        """Main loop for a worker thread."""
        while not self._shutdown_event.is_set():
            try:
                item = self._request_queue.get(timeout=1.0)
            except Exception:
                continue

            try:
                context = process_fn(item, services, worker_id)
                self._result_queue.put_nowait(context)
            except Exception:
                self._result_queue.put_nowait({
                    "error": f"Worker {worker_id} fatal: {traceback.format_exc()}",
                    "worker_id": worker_id,
                })

    def shutdown(self, timeout=10):
        """Graceful shutdown: signal workers, wait for completion."""
        with self._lock:
            self._shutdown_event.set()
            self._running = False

        for worker in self._workers:
            worker.join(timeout=timeout)

        self._workers.clear()

    @property
    def running(self):
        return self._running

    @property
    def worker_count(self):
        return self._num_workers

    @property
    def alive_workers(self):
        return sum(1 for w in self._workers if w.is_alive())
