use crate::chat::ChatState;
use axum::{extract::State, http::StatusCode, response::{Html, IntoResponse}};
use std::sync::Arc;

const INDEX_HTML: &str = include_str!("../static/index.html");
const CHAT_JS: &str = include_str!("../static/chat.js");
const CHAT_CSS: &str = include_str!("../static/chat.css");

pub async fn index_handler(State(state): State<Arc<ChatState>>) -> impl IntoResponse {
    let nickname = state.allocate_nickname();

    let messages = state.get_last_n(10);
    let messages_html: String = messages
        .iter()
        .map(|msg| {
            format!(
                r#"<div class="msg" data-id="{}"><span class="nick">{}</span><span class="text">{}</span><time>{}</time></div>
"#,
                msg.id,
                msg.nickname,
                msg.text,
                msg.timestamp.format("%H:%M:%S%.3f")
            )
        })
        .collect();

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
            ("Cache-Control", "public, max-age=3600, immutable"),
        ],
        CHAT_JS,
    )
}

pub async fn chat_css_handler() -> impl IntoResponse {
    (
        StatusCode::OK,
        [
            ("Content-Type", "text/css; charset=utf-8"),
            ("Cache-Control", "public, max-age=3600, immutable"),
        ],
        CHAT_CSS,
    )
}

pub async fn favicon_handler() -> impl IntoResponse {
    StatusCode::NOT_FOUND
}
