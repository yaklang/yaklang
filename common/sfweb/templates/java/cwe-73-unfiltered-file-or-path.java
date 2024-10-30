package com.example.filedownload;
import java.io.File;
@RestController
public class FileDownloadController {

    @GetMapping("/download/{filename}")
    public ResponseEntity<FileSystemResource> downloadFile(@PathVariable String filename) {
        // 指定文件的路径
        File file = new File("path/to/your/files/" + filename);

        if (!file.exists()) {
            return ResponseEntity.status(HttpStatus.NOT_FOUND).build();
        }

        // 设置响应头
        HttpHeaders headers = new HttpHeaders();
        headers.add(HttpHeaders.CONTENT_DISPOSITION, "attachment; filename=" + file.getName());

        // 返回文件
        return ResponseEntity.ok()
                .headers(headers)
                .body(new FileSystemResource(file));
    }
}