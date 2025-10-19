use std::path::Path;

#[tokio::main]
async fn main() {
    let test_file = "../test-logs/test-rust.jsonl";
    
    println!("Testing archive function...");
    
    let metadata = tokio::fs::metadata(test_file).await.unwrap();
    println!("Before: File size = {} bytes", metadata.len());
    
    fast_chat_server::chat::archive_log_file(test_file).await.unwrap();
    
    let metadata = tokio::fs::metadata(test_file).await.unwrap();
    println!("After: File size = {} bytes", metadata.len());
    
    let entries = std::fs::read_dir("../test-logs").unwrap();
    let archives: Vec<_> = entries
        .filter_map(|e| e.ok())
        .filter(|e| e.path().to_string_lossy().contains("test-rust") && e.path().to_string_lossy().ends_with(".gz"))
        .collect();
    
    println!("Archive files created: {}", archives.len());
    for entry in archives {
        let metadata = entry.metadata().unwrap();
        println!("  - {} ({} bytes)", entry.file_name().to_string_lossy(), metadata.len());
    }
    
    println!("Test completed successfully!");
}
