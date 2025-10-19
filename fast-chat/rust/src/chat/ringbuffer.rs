use super::message::Message;
use std::sync::Arc;

pub struct RingBuffer {
    buffer: Vec<Option<Arc<Message>>>,
    size: usize,
    head: usize,
    tail: usize,
    count: usize,
}

impl RingBuffer {
    pub fn new(capacity: usize) -> Self {
        Self {
            buffer: (0..capacity).map(|_| None).collect(),
            size: capacity,
            head: 0,
            tail: 0,
            count: 0,
        }
    }

    pub fn push(&mut self, msg: Message) {
        self.buffer[self.head] = Some(Arc::new(msg));
        self.head = (self.head + 1) % self.size;

        if self.count < self.size {
            self.count += 1;
        } else {
            self.tail = (self.tail + 1) % self.size;
        }
    }

    pub fn get_last(&self, n: usize) -> Vec<Arc<Message>> {
        if self.count == 0 {
            return vec![];
        }

        let n = n.min(self.count);
        let mut result = Vec::with_capacity(n);
        let start_pos = (self.head + self.size - n) % self.size;

        for i in 0..n {
            let pos = (start_pos + i) % self.size;
            if let Some(ref msg) = self.buffer[pos] {
                result.push(Arc::clone(msg));
            }
        }

        result
    }

    pub fn get_history(&self, skip: usize, take: usize) -> Vec<Arc<Message>> {
        if self.count == 0 {
            return vec![];
        }

        let end = self.count.saturating_sub(skip);
        if end == 0 {
            return vec![];
        }

        let start = end.saturating_sub(take);
        let actual_take = end - start;
        let mut result = Vec::with_capacity(actual_take);

        let start_pos = (self.head + self.size - self.count + start) % self.size;

        for i in 0..actual_take {
            let pos = (start_pos + i) % self.size;
            if let Some(ref msg) = self.buffer[pos] {
                result.push(Arc::clone(msg));
            }
        }

        result
    }

    #[allow(dead_code)]
    pub fn count(&self) -> usize {
        self.count
    }

    #[allow(dead_code)]
    pub fn capacity(&self) -> usize {
        self.size
    }
}
