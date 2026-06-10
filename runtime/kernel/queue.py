"""AI Workstation — Execution Queue Layer (v0.4.0)

Thread-safe request queue with backpressure and concurrency guard.
Replaces direct execution with controlled dispatch.

Properties:
  - FIFO ordering (deterministic dispatch)
  - Backpressure: max queue size; reject when full
  - Max concurrency: configurable worker count
  - Timeout: configurable get() timeout
  - Metrics: enqueued, dequeued, rejected, queue_depth
"""

from __future__ import annotations

import threading
import time
from typing import Optional


class QueueFullError(Exception):
    """Raised when the queue is at capacity and backpressure rejects a request."""


class QueueEmptyError(Exception):
    """Raised when get() times out waiting for a request."""


class RequestQueue:
    """Bounded FIFO request queue with backpressure.

    Thread-safe. Multiple producers (stdin reader) and multiple consumers
    (worker threads) can operate concurrently.

    Metrics are exposed for observability (queue depth, rejection count).
    """

    def __init__(self, max_size=128, get_timeout=30.0):
        self._max_size = max_size
        self._get_timeout = get_timeout
        self._queue = []
        self._lock = threading.Lock()
        self._not_empty = threading.Condition(self._lock)
        self._not_full = threading.Condition(self._lock)

        self._metrics = {
            "enqueued": 0,
            "dequeued": 0,
            "rejected": 0,
            "timed_out": 0,
        }

    def put(self, item, block=True, timeout=None):
        """Enqueue a request item.

        Args:
            item: request context or raw request dict
            block: if True, wait if queue is full
            timeout: max wait time in seconds

        Returns:
            True if enqueued, False if timed out

        Raises:
            QueueFullError if not blocking and queue is full
        """
        with self._not_full:
            if not block and len(self._queue) >= self._max_size:
                self._metrics["rejected"] += 1
                raise QueueFullError(
                    f"Queue full ({self._max_size} max). Backpressure active."
                )

            if block:
                deadline = time.time() + (timeout or self._get_timeout)
                while len(self._queue) >= self._max_size:
                    remaining = deadline - time.time()
                    if remaining <= 0:
                        self._metrics["rejected"] += 1
                        return False
                    self._not_full.wait(timeout=remaining)

            self._queue.append(item)
            self._metrics["enqueued"] += 1
            self._not_empty.notify()
            return True

    def get(self, timeout=None):
        """Dequeue a request item (blocking).

        Args:
            timeout: max wait time in seconds

        Returns:
            request item

        Raises:
            QueueEmptyError if timeout expires
        """
        with self._not_empty:
            deadline = time.time() + (timeout or self._get_timeout)
            while not self._queue:
                remaining = deadline - time.time()
                if remaining <= 0:
                    self._metrics["timed_out"] += 1
                    raise QueueEmptyError(
                        f"No request available after {timeout or self._get_timeout}s"
                    )
                self._not_empty.wait(timeout=min(remaining, 1.0))

            item = self._queue.pop(0)
            self._metrics["dequeued"] += 1
            self._not_full.notify()
            return item

    def get_nowait(self):
        """Dequeue without blocking. Returns None if empty."""
        with self._lock:
            if not self._queue:
                return None
            item = self._queue.pop(0)
            self._metrics["dequeued"] += 1
            return item

    def drain(self):
        """Flush all pending items and return them (shutdown)."""
        with self._lock:
            items = list(self._queue)
            self._queue.clear()
            return items

    @property
    def size(self):
        """Current queue depth."""
        with self._lock:
            return len(self._queue)

    @property
    def metrics(self):
        """Return a snapshot of queue metrics."""
        with self._lock:
            m = dict(self._metrics)
            m["queue_depth"] = len(self._queue)
            m["max_size"] = self._max_size
            return m

    @property
    def is_full(self):
        with self._lock:
            return len(self._queue) >= self._max_size

    @property
    def is_empty(self):
        with self._lock:
            return len(self._queue) == 0
