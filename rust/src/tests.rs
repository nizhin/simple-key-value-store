use super::*;
use axum::{
    body::{Body, to_bytes},
    http::Request,
};
use tower::ServiceExt;

#[tokio::test]
async fn operations_and_errors() {
    let app = handle_request.with_state(new_store());
    let steps = [
        ("GET", "/name", "", 404, None),
        ("PUT", "/name", r#"{"value":"Tien"}"#, 204, None),
        ("GET", "/name", "", 200, Some("Tien")),
        ("POST", "/name", r#"{"value":"updated"}"#, 204, None),
        ("GET", "/name", "", 200, Some("updated")),
        ("PUT", "/name", "not JSON", 400, None),
        ("PUT", "/name", "{}", 400, None),
        ("PUT", "/name", r#"{"value":null}"#, 400, None),
        ("PUT", "/name", r#"{"value":123}"#, 400, None),
        ("GET", "/name", "", 200, Some("updated")),
        ("DELETE", "/name", "", 204, None),
        ("GET", "/name", "", 404, None),
        ("DELETE", "/name", "", 404, None),
        ("POST", "/name", r#"{"value":""}"#, 204, None),
        ("GET", "/name", "", 200, Some("")),
        ("GET", "/", "", 400, None),
        ("PATCH", "/name", "", 405, None),
    ];

    for (method, path, body, status, expected) in steps {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method(method)
                    .uri(path)
                    .body(Body::from(body))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status().as_u16(), status, "{method} {path}");
        if status == 405 {
            assert_eq!(response.headers()[header::ALLOW], "GET, PUT, POST, DELETE");
        }
        if let Some(value) = expected {
            let bytes = to_bytes(response.into_body(), 1024).await.unwrap();
            let payload: ValueBody = serde_json::from_slice(&bytes).unwrap();
            assert_eq!(payload.value, value);
        }
    }
}
