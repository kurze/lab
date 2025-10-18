use super::message::Message;
use anyhow::Result;
use std::path::Path;
use tokio::fs::{File, OpenOptions};
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::sync::mpsc;

pub struct MessageLogger {
    sender: mpsc::Sender<Message>,
}

impl MessageLogger {
    pub async fn new(filename: &str, buffer_size: usize) -> Result<Self> {
        let file = OpenOptions::new()
            .create(true)
            .write(true)
            .append(true)
            .open(filename)
            .await?;

        let (sender, receiver) = mpsc::channel(buffer_size);

        tokio::spawn(write_loop(file, receiver));

        Ok(Self { sender })
    }

    pub fn log(&self, msg: Message) {
        let _ = self.sender.try_send(msg);
    }
}

async fn write_loop(mut file: File, mut receiver: mpsc::Receiver<Message>) {
    while let Some(msg) = receiver.recv().await {
        if let Ok(json) = serde_json::to_string(&msg) {
            let _ = file.write_all(json.as_bytes()).await;
            let _ = file.write_all(b"\n").await;
        }
    }
    
    let _ = file.flush().await;
}

pub async fn load_messages(filename: &str) -> Result<Vec<Message>> {
    if !Path::new(filename).exists() {
        return Ok(Vec::new());
    }

    let file = File::open(filename).await?;
    let reader = BufReader::new(file);
    let mut lines = reader.lines();
    let mut messages = Vec::new();

    while let Some(line) = lines.next_line().await? {
        if line.trim().is_empty() {
            continue;
        }
        if let Ok(msg) = serde_json::from_str::<Message>(&line) {
            messages.push(msg);
        }
    }

    Ok(messages)
}
