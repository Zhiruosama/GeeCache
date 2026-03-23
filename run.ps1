go build -o server.exe

$p1 = Start-Process -FilePath "./server.exe" -ArgumentList "-port=8001" -PassThru -NoNewWindow
$p2 = Start-Process -FilePath "./server.exe" -ArgumentList "-port=8002" -PassThru -NoNewWindow
$p3 = Start-Process -FilePath "./server.exe" -ArgumentList "-port=8003","-api=1" -PassThru -NoNewWindow

Start-Sleep -Seconds 2

Write-Host ">>> start test"
Invoke-WebRequest -Uri "http://localhost:9999/api?key=Tom" -UseBasicParsing | Select-Object -ExpandProperty Content
Invoke-WebRequest -Uri "http://localhost:9999/api?key=Tom" -UseBasicParsing | Select-Object -ExpandProperty Content
Invoke-WebRequest -Uri "http://localhost:9999/api?key=Tom" -UseBasicParsing | Select-Object -ExpandProperty Content

# 清理
$p1, $p2, $p3 | Stop-Process -Force
Start-Sleep -Seconds 1
Remove-Item "./server.exe" -Force -ErrorAction SilentlyContinue