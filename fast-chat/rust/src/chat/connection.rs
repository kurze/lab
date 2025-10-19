use std::sync::atomic::{AtomicBool, AtomicI64, Ordering};
use std::sync::Arc;
use tokio::sync::mpsc;
use uuid::Uuid;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[allow(dead_code)]
pub enum TransportType {
    WebSocket,
    WebTransport,
}

#[derive(Clone)]
pub struct Connection {
    pub id: Uuid,
    nickname: Arc<parking_lot::RwLock<String>>,
    #[allow(dead_code)]
    pub transport: TransportType,
    last_seen: Arc<AtomicI64>,
    pub send_chan: mpsc::Sender<Arc<String>>,
    closed: Arc<AtomicBool>,
}

impl Connection {
    pub fn new(transport: TransportType, buffer_size: usize) -> (Self, mpsc::Receiver<Arc<String>>) {
        let (sender, receiver) = mpsc::channel(buffer_size);
        let now = chrono::Utc::now().timestamp_nanos_opt().unwrap_or(0);
        
        let conn = Self {
            id: Uuid::new_v4(),
            nickname: Arc::new(parking_lot::RwLock::new(String::new())),
            transport,
            last_seen: Arc::new(AtomicI64::new(now)),
            send_chan: sender,
            closed: Arc::new(AtomicBool::new(false)),
        };

        (conn, receiver)
    }

    pub fn set_nickname(&self, nickname: String) {
        *self.nickname.write() = nickname;
    }

    pub fn get_nickname(&self) -> String {
        self.nickname.read().clone()
    }

    pub fn update_last_seen(&self) {
        let now = chrono::Utc::now().timestamp_nanos_opt().unwrap_or(0);
        self.last_seen.store(now, Ordering::Relaxed);
    }

    pub fn send(&self, msg: Arc<String>) -> bool {
        if self.is_closed() {
            return false;
        }

        match self.send_chan.try_send(msg.clone()) {
            Ok(_) => true,
            Err(mpsc::error::TrySendError::Full(msg)) => {
                let send_chan = self.send_chan.clone();
                tokio::spawn(async move {
                    let _ = send_chan.send(msg).await;
                });
                true
            }
            Err(mpsc::error::TrySendError::Closed(_)) => false,
        }
    }

    pub fn close(&self) {
        self.closed.store(true, Ordering::Relaxed);
    }

    pub fn is_closed(&self) -> bool {
        self.closed.load(Ordering::Relaxed)
    }
}
