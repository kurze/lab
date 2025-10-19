use super::connection::Connection;
use super::logger::{archive_log_file, load_messages, MessageLogger};
use super::message::Message;
use super::nicknames::NicknamePool;
use super::ringbuffer::RingBuffer;
use anyhow::Result;
use parking_lot::RwLock;
use std::collections::HashMap;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::Arc;
use std::time::Duration;
use uuid::Uuid;

pub struct ChatState {
    messages: Arc<RwLock<RingBuffer>>,
    connections: Arc<RwLock<HashMap<Uuid, Connection>>>,
    nickname_pool: Arc<RwLock<NicknamePool>>,
    logger: Arc<MessageLogger>,
    next_id: AtomicI64,
}

impl ChatState {
    pub async fn new(log_file: &str, max_messages: usize, log_buffer_size: usize, flush_interval: Duration) -> Result<Self> {
        let loaded_messages = load_messages(log_file).await?;

        if let Err(e) = archive_log_file(log_file).await {
            tracing::warn!("Failed to archive log file: {}", e);
        }

        let logger = MessageLogger::new(log_file, log_buffer_size, flush_interval).await?;

        let total_loaded = loaded_messages.len();
        if total_loaded == 0 {
            tracing::info!("No previous messages found, starting fresh");
        } else {
            tracing::info!("Loaded {} messages from log file", total_loaded);
        }

        let mut ring_buffer = RingBuffer::new(max_messages);
        let mut next_id = 0i64;

        if !loaded_messages.is_empty() {
            let start = if total_loaded > max_messages {
                tracing::info!(
                    "Keeping last {} messages in memory (discarding {} older messages)",
                    max_messages,
                    total_loaded - max_messages
                );
                total_loaded - max_messages
            } else {
                0
            };

            for msg in &loaded_messages[start..] {
                ring_buffer.push(msg.clone());
            }

            next_id = loaded_messages.last().unwrap().id;
            tracing::info!("Resuming message IDs from {}", next_id);
        }

        Ok(Self {
            messages: Arc::new(RwLock::new(ring_buffer)),
            connections: Arc::new(RwLock::new(HashMap::new())),
            nickname_pool: Arc::new(RwLock::new(NicknamePool::new())),
            logger: Arc::new(logger),
            next_id: AtomicI64::new(next_id),
        })
    }

    pub fn add_message(&self, nickname: String, text: String) -> Message {
        let id = self.next_id.fetch_add(1, Ordering::SeqCst) + 1;
        let msg = Message::new(id, nickname, text);

        self.messages.write().push(msg.clone());

        self.logger.log(msg.clone());

        msg
    }

    pub fn get_last_n(&self, n: usize) -> Vec<Arc<Message>> {
        self.messages.read().get_last(n)
    }

    pub fn get_history(&self, skip: usize, take: usize) -> Vec<Arc<Message>> {
        self.messages.read().get_history(skip, take)
    }

    pub fn add_connection(&self, conn: Connection) {
        self.connections.write().insert(conn.id, conn);
    }

    pub fn remove_connection(&self, id: Uuid) -> Option<String> {
        let mut connections = self.connections.write();
        
        if let Some(conn) = connections.get(&id) {
            if conn.is_closed() {
                connections.remove(&id);
                return None;
            }

            let nickname = conn.get_nickname();
            conn.close();
            connections.remove(&id);

            if !nickname.is_empty() {
                self.nickname_pool.write().release(nickname.clone());
                return Some(nickname);
            }
        }

        None
    }

    #[allow(dead_code)]
    pub fn get_connection(&self, id: Uuid) -> Option<Connection> {
        self.connections.read().get(&id).cloned()
    }

    pub fn connection_count(&self) -> usize {
        self.connections.read().len()
    }

    pub fn broadcast(&self, msg: String) {
        let msg = Arc::new(msg);
        let connections = self.connections.read();
        
        for conn in connections.values() {
            if !conn.is_closed() {
                conn.send(Arc::clone(&msg));
            }
        }
    }

    pub fn broadcast_except(&self, msg: String, except_id: Uuid) {
        let msg = Arc::new(msg);
        let connections = self.connections.read();
        
        for (id, conn) in connections.iter() {
            if *id != except_id && !conn.is_closed() {
                conn.send(Arc::clone(&msg));
            }
        }
    }

    pub fn allocate_nickname(&self) -> String {
        self.nickname_pool.write().allocate()
    }

    #[allow(dead_code)]
    pub fn nickname_pool_stats(&self) -> (usize, usize) {
        let pool = self.nickname_pool.read();
        (pool.available(), pool.used())
    }
}
