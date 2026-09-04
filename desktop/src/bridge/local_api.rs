use serde_json::Value;
use std::io::{Read, Write};

trait ReadWrite: Read + Write {}
impl<T: Read + Write> ReadWrite for T {}

#[cfg(unix)]
fn connect() -> Result<Box<dyn ReadWrite>, String> {
    let path = "/var/lib/redflag/agent/localapi/redflag-agent.sock";
    std::os::unix::net::UnixStream::connect(path)
        .map(|stream| Box::new(stream) as Box<dyn ReadWrite>)
        .map_err(|error| format!("connect RedFlag Agent at {path}: {error}"))
}

#[cfg(windows)]
fn connect() -> Result<Box<dyn ReadWrite>, String> {
    let path = r"\\.\pipe\RedFlagAgentLocal";
    std::fs::OpenOptions::new()
        .read(true)
        .write(true)
        .open(path)
        .map(|stream| Box::new(stream) as Box<dyn ReadWrite>)
        .map_err(|error| format!("connect RedFlag Agent at {path}: {error}"))
}

pub fn get(path: &str) -> Result<Value, String> {
    request("GET", path, None)
}

/// Ok(None) when the Agent has no such route. An older Agent is not an
/// unreachable one, and rendering it as a connection failure is a lie about
/// which half of the pair is behind.
pub fn get_optional(path: &str) -> Result<Option<Value>, String> {
    match raw("GET", path, None) {
        Ok((404, _)) => Ok(None),
        Ok((status, body)) => decode(status, &body).map(Some),
        Err(error) => Err(error),
    }
}

pub fn post(path: &str, value: &Value) -> Result<Value, String> {
    let body =
        serde_json::to_vec(value).map_err(|error| format!("encode Agent request: {error}"))?;
    request("POST", path, Some(&body))
}

fn request(method: &str, path: &str, body: Option<&[u8]>) -> Result<Value, String> {
    let (status, body) = raw(method, path, body)?;
    decode(status, &body)
}

fn decode(status: u16, body: &str) -> Result<Value, String> {
    if !(200..300).contains(&status) {
        if let Ok(value) = serde_json::from_str::<Value>(body) {
            if let Some(error) = value["error"].as_str() {
                return Err(error.to_string());
            }
        }
        return Err(format!("Agent returned {status}: {}", body.trim()));
    }
    serde_json::from_str(body).map_err(|error| format!("decode Agent response: {error}"))
}

fn raw(method: &str, path: &str, body: Option<&[u8]>) -> Result<(u16, String), String> {
    let body = body.unwrap_or_default();
    let mut stream = connect()?;
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: redflag.local\r\nContent-Type: application/json\r\nAccept: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        body.len()
    );
    stream
        .write_all(request.as_bytes())
        .map_err(|error| format!("write Agent request: {error}"))?;
    stream
        .write_all(body)
        .map_err(|error| format!("write Agent body: {error}"))?;
    stream
        .flush()
        .map_err(|error| format!("flush Agent request: {error}"))?;

    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .map_err(|error| format!("read Agent response: {error}"))?;
    let (head, body) = response
        .split_once("\r\n\r\n")
        .ok_or_else(|| "Agent response has no HTTP body".to_string())?;
    let status = head
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|code| code.parse::<u16>().ok())
        .ok_or_else(|| "Agent response has no status".to_string())?;
    Ok((status, body.to_string()))
}
