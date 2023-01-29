# radiobuenavia
Automate audacity mixing and uploading to dropbox

```
pip install .
rbv
```

# Generating Refresh Tokens

Go here and access your app:
```
https://www.dropbox.com/developers/apps
```

Copy the `App key` then paste into and execute the below URL:

```
https://www.dropbox.com/oauth2/authorize?client_id=$app_key&token_access_type=offline&response_type=code
```

Generate an `App secret` from the app developers menu and paste in that plus the `App key` and `generated_code` from the previous URL:
```
curl --location --request POST 'https://api.dropboxapi.com/oauth2/token' \
-u '$app_key:$app_secret' \
-H 'Content-Type: application/x-www-form-urlencoded' \
--data-urlencode 'code=$generated_code' \
--data-urlencode 'grant_type=authorization_code'
```

Paste the `App secret`, `App key` and the `refresh_token` in the response into the `config.toml`.

# FFMPEG
This tool uses pydub which has a dependency on ffmpeg. This is due to being unable to script bitrates and editing the metadata through audacity scripting.
