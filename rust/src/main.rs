use std::{
    collections::HashMap,
    sync::{Arc, Mutex},
};

use axum::{
    body::Bytes,
    extract::State,
    handler::Handler,
    http::{header, Method, StatusCode, Uri},
    response::{IntoResponse, Response},
    serve::ListenerExt,
    Json,
};
use serde::{Deserialize, Serialize};
use tokio::net::TcpListener;

type Store = Arc<Mutex<HashMap<String, String>>>;

#[derive(Deserialize, Serialize)]
struct ValueBody {
    value: String,
}

#[derive(Serialize)]
struct ValueBodyRef<'a> {
    value: &'a str,
}

fn new_store() -> Store {
    Arc::new(Mutex::new(HashMap::with_capacity(16)))
}

async fn handle_request(
    State(store): State<Store>,
    method: Method,
    uri: Uri,
    body: Bytes,
) -> Response {
    let key = uri.path().strip_prefix('/').unwrap_or_default();
    if key.is_empty() {
        return (StatusCode::BAD_REQUEST, "key is required\n").into_response();
    }

    match method {
        Method::GET => {
            let entries = store.lock().expect("store lock poisoned");
            match entries.get(key) {
                Some(value) => Json(ValueBodyRef { value: value.as_str() }).into_response(),
                None => (StatusCode::NOT_FOUND, "key not found\n").into_response(),
            }
        }
        Method::PUT | Method::POST => {
            let Ok(payload) = serde_json::from_slice::<ValueBody>(&body) else {
                return (
                    StatusCode::BAD_REQUEST,
                    "expected JSON with a string value\n",
                )
                    .into_response();
            };

            store
                .lock()
                .expect("store lock poisoned")
                .insert(key.to_owned(), payload.value);

            StatusCode::NO_CONTENT.into_response()
        }
        Method::DELETE => {
            let removed = store.lock().expect("store lock poisoned").remove(key);
            if removed.is_some() {
                StatusCode::NO_CONTENT.into_response()
            } else {
                (StatusCode::NOT_FOUND, "key not found\n").into_response()
            }
        }
        _ => (
            StatusCode::METHOD_NOT_ALLOWED,
            [(header::ALLOW, "GET, PUT, POST, DELETE")],
            "method not allowed\n",
        )
            .into_response(),
    }
}


#[tokio::main(flavor = "current_thread")]
async fn main() -> std::io::Result<()> {
    let listener = TcpListener::bind("127.0.0.1:8080").await?.tap_io(|stream| {
        if let Err(error) = stream.set_nodelay(true) {
            eprintln!("Failed to enable TCP_NODELAY: {error}");
        }
    });
    eprintln!("Server listening on http://127.0.0.1:8080");
    let service = handle_request.with_state(new_store());
    axum::serve(listener, service.into_make_service()).await
}

#[cfg(test)]
mod tests;
