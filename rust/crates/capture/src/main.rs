use std::env;
use std::fs;
use std::io::{self, BufRead, Write};

const HELP: &str = "\
apishadow-capture — нормализация зеркального трафика

  apishadow-capture version
  apishadow-capture normalize  <file.jsonl>
  apishadow-capture fingerprint <file.jsonl>
  apishadow-capture demo
";

fn process_line(line: &str, mode: &str) -> String {
    match mode {
        "normalize" => apishadow_capture::normalize(line).unwrap_or_else(|e| format!("{{\"error\":{}}}", apishadow_capture::json_escape(&e))),
        "fingerprint" => apishadow_capture::fingerprint(line).unwrap_or_else(|e| format!("{{\"error\":{}}}", apishadow_capture::json_escape(&e))),
        _ => "{}".into(),
    }
}

fn main() {
    let args: Vec<String> = env::args().skip(1).collect();
    if args.is_empty() || args[0] == "help" {
        print!("{}", HELP);
        return;
    }
    match args[0].as_str() {
        "version" => println!("{}", apishadow_capture::VERSION),
        "demo" => {
            let sample = r#"{"id":"demo","role":"primary","ts":1,"method":"GET","path":"/v1/users","query":"","status":200,"duration_ms":10,"headers":{"Content-Type":"application/json","Date":"now"},"body":"{\"ok\":true}"}"#;
            println!("{}", process_line(sample, "normalize"));
        }
        mode @ ("normalize" | "fingerprint") => {
            let Some(path) = args.get(1) else {
                eprintln!("нужен файл.jsonl");
                std::process::exit(2);
            };
            let file = match fs::File::open(path) {
                Ok(file) => file,
                Err(err) => {
                    eprintln!("не удалось открыть {}: {}", path, err);
                    std::process::exit(1);
                }
            };
            let stdout = io::stdout();
            let mut out = io::BufWriter::new(stdout.lock());
            for line in io::BufReader::new(file).lines().map_while(Result::ok) {
                if line.trim().is_empty() {
                    continue;
                }
                let _ = writeln!(out, "{}", process_line(&line, mode));
            }
        }
        other => {
            eprintln!("неизвестная команда: {}", other);
            print!("{}", HELP);
            std::process::exit(2);
        }
    }
}
