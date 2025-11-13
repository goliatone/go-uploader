# Chunked Upload Demo

This example drives the chunked/resumable APIs end-to-end so you can mirror the lifecycle in your own UI.

## Run the CLI
```bash
go run ./examples/chunked
```

The program:
1. Creates `./.example-chunks` as the filesystem provider root.
2. Initiates a chunk session via `Manager.InitiateChunked`.
3. Streams data in 1 KB parts with `UploadChunk`.
4. Calls `CompleteChunked` and prints the resulting URL + size.

Inspect the staged files to see how chunk metadata is persisted:
```bash
find .example-chunks -maxdepth 2 -type f
```

## Wire into a Browser UI
When exposing these endpoints over HTTP, your front-end can follow this rough flow:

```js
async function uploadInChunks(file) {
  const init = await fetch("/api/uploads/chunked", {
    method: "POST",
    body: JSON.stringify({ key: `videos/${file.name}`, size: file.size })
  }).then(r => r.json());

  const chunkSize = init.part_size;
  let partIndex = 0;
  for (let offset = 0; offset < file.size; offset += chunkSize) {
    const body = file.slice(offset, offset + chunkSize);
    await fetch(`/api/uploads/chunked/${init.session_id}/${partIndex}`, {
      method: "PUT",
      body,
    });
    partIndex++;
  }

  return fetch(`/api/uploads/chunked/${init.session_id}/complete`, { method: "POST" })
    .then(r => r.json());
}
```

Pair this UI sketch with the CLI behaviour above to validate chunk ordering, retry semantics, and cleanup logic before hitting production storage.
