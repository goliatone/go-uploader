# Presigned Upload Form Demo

Use this example to generate POST policies that let browsers upload directly to storage without proxying bytes through your API.

## Run the generator
```bash
go run ./examples/presignedpost
```

The CLI prints:
1. `post.URL` – the form action for your storage provider.
2. `post.Fields` – hidden inputs (policy, signature, key, etc.).
3. Confirmation output from `Manager.ConfirmPresignedUpload`, proving the metadata workflow succeeds.

## Drop the payload into a form
Copy the contents of `form.html`, paste the generated field values, and open it in a browser:

```html
<form action="https://upload.example.com/form" method="post" enctype="multipart/form-data">
  <input type="hidden" name="key" value="uploads/demo.txt">
  <input type="hidden" name="token" value="demo-token">
  <input type="file" name="file" />
  <button>Upload</button>
</form>
```

After the upload succeeds, hit your API endpoint that wraps `ConfirmPresignedUpload` so file metadata (size, content type, URL) lands in persistent storage.

## Testing unsupported providers
Providers that do not implement `PresignedPoster` automatically return `ErrNotImplemented`. Keep this example handy when building custom providers so you can quickly validate field contents and error paths before exposing them to clients.
