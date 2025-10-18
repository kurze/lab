use parking_lot::Mutex;
use super::message::Message;

pub struct RingBuffer {
    buffer: Vec<Option<Message>>,
    size: usize,
    head: usize,
    tail: usize,
    count: usize,
    mu: Mutex<()>,
}

impl RingBuffer {
    pub fn new(capacity: usize) -> Self {
        Self {
            buffer: (0..capacity).map(|_| None).collect(),
            size: capacity,
            head: 0,
            tail: 0,
            count: 0,
            mu: Mutex::new(()),
        }
    }

    pub fn push(&mut self, msg: Message) {
        let _lock = self.mu.lock();
        
        self.buffer[self.head] = Some(msg);
        self.head = (self.head + 1) % self.size;

        if self.count < self.size {
            self.count += 1;
        } else {
            self.tail = (self.tail + 1) % self.size;
        }
    }

    pub fn get_last(&self, n: usize) -> Vec<Message> {
        let _lock = self.mu.lock();

        if self.count == 0 {
            return vec![];
        }

        let n = n.min(self.count);
        let mut result = Vec::with_capacity(n);
        let start_pos = (self.head + self.size - n) % self.size;

        for i in 0..n {
            let pos = (start_pos + i) % self.size;
            if let Some(ref msg) = self.buffer[pos] {
                result.push(msg.clone());
            }
        }

        result
    }

    pub fn get_history(&self, skip: usize, take: usize) -> Vec<Message> {
        let _lock = self.mu.lock();

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
                result.push(msg.clone());
            }
        }

        result
    }

    pub fn count(&self) -> usize {
        let _lock = self.mu.lock();
        self.count
    }

    pub fn capacity(&self) -> usize {
        self.size
    }
}
