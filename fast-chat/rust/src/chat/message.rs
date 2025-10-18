use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub id: i64,
    pub nickname: String,
    pub text: String,
    pub timestamp: DateTime<Utc>,
}

impl Message {
    pub fn new(id: i64, nickname: String, text: String) -> Self {
        Self {
            id,
            nickname: html_escape::encode_text(&nickname).to_string(),
            text: html_escape::encode_text(&text).to_string(),
            timestamp: Utc::now(),
        }
    }

    pub fn to_json(&self) -> String {
        let msg = ClientMessage {
            r#type: "message".to_string(),
            action: "append".to_string(),
            timestamp: Utc::now().timestamp_millis(),
            data: serde_json::json!(MessageData {
                id: self.id.to_string(),
                nickname: self.nickname.clone(),
                text: self.text.clone(),
                time: self.timestamp.format("%H:%M:%S%.3f").to_string(),
                is_system: None,
                client_id: None,
                server_id: None,
            }),
        };

        serde_json::to_string(&msg).unwrap_or_else(|_| {
            r#"{"type":"error","data":"Failed to encode message"}"#.to_string()
        })
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ClientMessage {
    pub r#type: String,
    pub action: String,
    pub data: serde_json::Value,
    pub timestamp: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MessageData {
    pub id: String,
    pub nickname: String,
    pub text: String,
    pub time: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    #[serde(rename = "isSystem")]
    pub is_system: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    #[serde(rename = "clientId")]
    pub client_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    #[serde(rename = "serverId")]
    pub server_id: Option<String>,
}

pub fn create_echo_message(msg: &Message, client_id: String) -> String {
    let echo_msg = ClientMessage {
        r#type: "message".to_string(),
        action: "append".to_string(),
        timestamp: Utc::now().timestamp_millis(),
        data: serde_json::json!(MessageData {
            id: msg.id.to_string(),
            nickname: msg.nickname.clone(),
            text: msg.text.clone(),
            time: msg.timestamp.format("%H:%M:%S%.3f").to_string(),
            is_system: None,
            client_id: Some(client_id),
            server_id: Some(msg.id.to_string()),
        }),
    };

    serde_json::to_string(&echo_msg).unwrap_or_else(|_| {
        r#"{"type":"error","data":"Failed to encode echo message"}"#.to_string()
    })
}

pub fn system_message_json(text: &str) -> String {
    let msg = ClientMessage {
        r#type: "message".to_string(),
        action: "append".to_string(),
        timestamp: Utc::now().timestamp_millis(),
        data: serde_json::json!(MessageData {
            id: String::new(),
            nickname: String::new(),
            text: html_escape::encode_text(text).to_string(),
            time: String::new(),
            is_system: Some(true),
            client_id: None,
            server_id: None,
        }),
    };

    serde_json::to_string(&msg).unwrap_or_else(|_| {
        r#"{"type":"error","data":"Failed to encode system message"}"#.to_string()
    })
}

pub fn user_count_json(count: usize) -> String {
    let plural = if count != 1 { "s" } else { "" };
    let text = format!("{} user{} online", count, plural);

    let msg = ClientMessage {
        r#type: "usercount".to_string(),
        action: "replace".to_string(),
        timestamp: Utc::now().timestamp_millis(),
        data: serde_json::json!(text),
    };

    serde_json::to_string(&msg).unwrap_or_else(|_| {
        r#"{"type":"error","data":"Failed to encode user count"}"#.to_string()
    })
}

pub fn history_messages_json(messages: Vec<&Message>) -> String {
    let history_data: Vec<MessageData> = messages
        .iter()
        .map(|msg| MessageData {
            id: msg.id.to_string(),
            nickname: msg.nickname.clone(),
            text: msg.text.clone(),
            time: msg.timestamp.format("%H:%M:%S%.3f").to_string(),
            is_system: None,
            client_id: None,
            server_id: None,
        })
        .collect();

    let msg = ClientMessage {
        r#type: "history".to_string(),
        action: "prepend".to_string(),
        timestamp: Utc::now().timestamp_millis(),
        data: serde_json::json!(history_data),
    };

    serde_json::to_string(&msg).unwrap_or_else(|_| {
        r#"{"type":"error","data":"Failed to encode history"}"#.to_string()
    })
}
