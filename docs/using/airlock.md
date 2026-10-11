# Moving data with the airlock

The airlock is the only way to bring files into a workspace or take results out. Each request
carries **one file**. Zip several files together if you need to.

## Importing a file

1. Open the workspace, then **Airlock**, then **New request**, and choose **Import**.
2. Give it a title and a business justification, and create it.
3. On the request, **Generate** a link. It is valid for an hour.
4. Upload your file to it from your own computer, with
   [Azure Storage Explorer](https://azure.microsoft.com/products/storage/storage-explorer/)
   (**Connect**, **Blob container**, **Shared access signature URL**) or with `azcopy`:

    ```bash
    azcopy copy ./data.zip "<link>"
    ```

5. **Submit** the request. You cannot change the file afterwards.
6. When it is approved you get an email. Inside a workspace VM, generate a link on the
   request and download the file:

    ```bash
    curl -o data.zip "$(echo '<link>' | sed 's#?#/data.zip?#')"
    ```

## Exporting a file

1. Create a request as above, and choose **Export**.
2. In a workspace VM, generate a link on the request and upload the file to it:

    ```bash
    curl -X PUT -H "x-ms-blob-type: BlockBlob" --upload-file results.zip \
      "$(echo '<link>' | sed 's#?#/results.zip?#')"
    ```

3. **Submit** it. When it is approved, download the file from outside the TRE with a link
   from the request.

The upload link only works from where the file is meant to come from: imports from outside
the TRE, exports from inside your workspace.

## When a request is not approved

| Status | Meaning |
|---|---|
| Blocked by scan | The malware scan found a threat. The file has been deleted. |
| Failed | The request had no file, or more than one. Create a new request. |
| Rejected | An airlock manager rejected it. Their explanation is on the request. |
| Revoked | An approved request was withdrawn. Its link no longer works. |

## Reviewing as an airlock manager

Requests waiting for you are listed under **Airlock** in the menu. On a request, either
generate a read-only link and inspect the file, or create a **review VM**, which has the link
ready in `~/airlock/` (Linux) or `C:\AzureData\CustomData.bin` (Windows). Then approve or
reject it with an explanation. Review VMs are removed once you decide.
