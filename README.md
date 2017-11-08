# swdeployer

## How to run it

```
go get github.com/EtienneBruines/swdeployer && go install github.com/EtienneBruines/swdeployer
```

## What's needed?
- You have to be within your plugin directory.
- You have to create a file `.shopware-deploy.ini`, with a value `plugin_id=1234` in there (with your plugin ID from Shopware).
- You need to have credentials in `~/.config/shopware-deploy`: first line is the username, second line the password

Now you run `swdeployer` in your plugin directory, and it'll guide you through the uploading process.
