# pnx

「作曲できる奴ちょっとこい」スレのまとめの一部を、ローカル環境で閲覧可能にするプログラムです。  

ダウンロードは以下の URL から行えます。

https://github.com/oov/pnx/releases

# 付属ファイルについて

Windows / Mac 向けに 32bit / 64bit 両方のバイナリが付属しており、付属しているデータベースは2013年7月当時のものです。  
（ただし Mac 向けバイナリは動作テストできていません）

ファイル名|説明
----------|----
`pnx-windows-4.0-386.exe`|Windows 用プログラム(32bit)
`pnx-windows-4.0-amd64.exe`|Windows 用プログラム(64bit)
`pnx-darwin-10.6-386.exe`|Mac 用プログラム(32bit)
`pnx-darwin-10.6-amd64.exe`|Mac 用プログラム(64bit)
`sdc.db/*`|2013年7月当時の楽曲データが保存されたデータベース(LevelDB)
`fts.db`|全文検索用データベース(SQLite / FTS4 / tri-gram)

# 使い方

ダブルクリックで起動して http://localhost:51123/ にアクセスするとページが表示できます。

リンク切れしている箇所も少なからずありますが、記憶と思い出で補完してください。
