use crate::chat::{create_echo_message, history_messages_json, system_message_json, user_count_json, ChatState, Connection, TransportType};
use axum::{
    extract::{
        ws::{Message, WebSocket},
        State, WebSocketUpgrade,
    },
    response::Response,
};
use futures::{sink::SinkExt, stream::StreamExt};
use std::sync::Arc;

pub async fn websocket_handler(
    ws: WebSocketUpgrade,
    State(state): State<Arc<ChatState>>,
) -> Response {
    ws.on_upgrade(|socket| handle_socket(socket, state))
}

async fn handle_socket(socket: WebSocket, state: Arc<ChatState>) {
    let (conn, mut receiver) = Connection::new(TransportType::WebSocket, 256);
    state.add_connection(conn.clone());

    tracing::info!("New WebSocket connection: {}", conn.id);

    let (mut ws_sender, mut ws_receiver) = socket.split();

    let conn_id = conn.id;
    let state_clone = state.clone();
    let send_task = tokio::spawn(async move {
        while let Some(msg) = receiver.recv().await {
            if ws_sender.send(Message::Text(msg)).await.is_err() {
                break;
            }
        }
    });

    while let Some(Ok(msg)) = ws_receiver.next().await {
        if let Message::Text(text) = msg {
            conn.update_last_seen();
            handle_client_message(&text, &conn, &state).await;
        } else if let Message::Close(_) = msg {
            break;
        }
    }

    send_task.abort();

    if let Some(nickname) = state_clone.remove_connection(conn_id) {
        let sys_msg = system_message_json(&format!("{} left the chat", nickname));
        state_clone.broadcast(sys_msg);

        let count = state_clone.connection_count();
        state_clone.broadcast(user_count_json(count));

        tracing::info!("WebSocket user {} disconnected ({})", nickname, conn_id);
    } else {
        tracing::info!("WebSocket connection already cleaned up: {}", conn_id);
    }
}

async fn handle_client_message(data: &str, conn: &Connection, state: &Arc<ChatState>) {
    let parts: Vec<&str> = data.split('|').collect();
    if parts.is_empty() {
        return;
    }

    let command = parts[0];

    match command {
        "JOIN" => {
            if parts.len() < 2 {
                return;
            }
            let nickname = parts[1].to_string();
            conn.set_nickname(nickname.clone());

            let sys_msg = system_message_json(&format!("{} joined the chat", nickname));
            state.broadcast(sys_msg);

            let count = state.connection_count();
            state.broadcast(user_count_json(count));

            tracing::info!("User {} joined (conn: {})", nickname, conn.id);
        }

        "SEND" => {
            if parts.len() < 3 {
                return;
            }
            let nickname = parts[1].to_string();
            let text = parts[2].to_string();
            let client_id = if parts.len() >= 4 {
                Some(parts[3].to_string())
            } else {
                None
            };

            let msg = state.add_message(nickname.clone(), text.clone());

            if let Some(client_id) = client_id {
                let echo_msg = create_echo_message(&msg, client_id);
                conn.send(echo_msg);
            }

            let json_msg = msg.to_json();
            state.broadcast_except(json_msg, conn.id);

            tracing::info!("Message from {}: {}", nickname, text);
        }

        "HISTORY" => {
            if parts.len() < 2 {
                return;
            }

            let (skip, take) = if parts.len() >= 3 {
                let skip = parts[1].parse::<usize>().unwrap_or(0);
                let take = parts[2].parse::<usize>().unwrap_or(10);
                (skip, take)
            } else {
                let take = parts[1].parse::<usize>().unwrap_or(10);
                (10, take)
            };

            let skip = skip.min(10000);
            let take = take.min(100);

            let messages = state.get_history(skip, take);

            if !messages.is_empty() {
                let history_refs: Vec<&crate::chat::Message> = messages.iter().collect();
                let history_json = history_messages_json(history_refs);
                conn.send(history_json);
            }
        }

        "PING" => {
        }

        _ => {}
    }
}
