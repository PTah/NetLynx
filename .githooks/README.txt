Хук commit-msg приводит trailer к единому виду: удаляет любые строки «Made-with: …» и добавляет:

  Made-with: Brain, Google and AI

Установка в клоне (копирует хук в .git/hooks и выставляет chmod +x при наличии Git Bash):

  powershell -ExecutionPolicy Bypass -File scripts/install-git-hooks.ps1

Либо вручную скопируйте .githooks/commit-msg в .git/hooks/commit-msg и сделайте файл исполняемым.
