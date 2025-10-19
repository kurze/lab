use crate::chat::ChatState;
use axum::{extract::State, http::StatusCode, response::{Html, IntoResponse}};
use std::sync::Arc;

const INDEX_HTML: &str = include_str!("../static/index.html");
const CHAT_JS_COMPRESSED: &[u8] = include_bytes!("../static/chat.js.gz");
const CHAT_CSS_COMPRESSED: &[u8] = include_bytes!("../static/chat.css.gz");

pub async fn index_handler(State(state): State<Arc<ChatState>>) -> impl IntoResponse {
    let nickname = state.allocate_nickname();

    let messages = state.get_last_n(10);
    let mut messages_html = String::with_capacity(messages.len() * 200);
    
    for msg in messages.iter() {
        messages_html.push_str(&format!(
            r#"<div class="msg" data-id="{}"><span class="nick">{}</span><span class="text">{}</span><time>{}</time></div>
"#,
            msg.id,
            msg.nickname,
            msg.text,
            msg.timestamp.format("%H:%M:%S%.3f")
        ));
    }

    let html = INDEX_HTML
        .replace("{{NICKNAME}}", &nickname)
        .replace("{{MESSAGES}}", &messages_html);

    Html(html)
}

pub async fn chat_js_handler() -> impl IntoResponse {
    (
        StatusCode::OK,
        [
            ("Content-Type", "application/javascript; charset=utf-8"),
            ("Cache-Control", "public, max-age=31536000, immutable"),
            ("Content-Encoding", "gzip"),
        ],
        CHAT_JS_COMPRESSED,
    )
}

pub async fn chat_css_handler() -> impl IntoResponse {
    (
        StatusCode::OK,
        [
            ("Content-Type", "text/css; charset=utf-8"),
            ("Cache-Control", "public, max-age=31536000, immutable"),
            ("Content-Encoding", "gzip"),
        ],
        CHAT_CSS_COMPRESSED,
    )
}

pub async fn favicon_handler() -> impl IntoResponse {
    StatusCode::NOT_FOUND
}
