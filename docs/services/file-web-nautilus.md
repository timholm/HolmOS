# File Web Nautilus

## Overview

File Web Nautilus is a GNOME Nautilus-inspired web-based file manager providing comprehensive file browsing, management, and storage configuration capabilities for the HolmOS cluster.

**Technology Stack:** Python Flask with PIL for image processing

**Default Port:** 5000

## Purpose

File Web Nautilus serves as the primary file management interface for HolmOS, offering:
- File and directory browsing with sorting and filtering
- File operations (upload, download, copy, move, rename, delete)
- Trash management with restore functionality
- Bookmarks and recent files tracking
- Image thumbnail generation
- Archive compression/decompression
- Storage configuration and mount point management

## UI Features

### Sidebar Navigation
- Default locations: Home, Documents, Downloads, Music, Pictures, Videos
- User-defined bookmarks
- Trash access
- Storage statistics display

### File Browser
- Grid and list view options
- Sorting by name, size, modified date, or type
- Hidden file toggle
- Breadcrumb navigation
- File/folder icons based on type and extension

### File Preview
- Text file content preview (up to 100KB)
- Image thumbnails with caching
- Video/PDF preview indicators

### Context Menu Operations
- Open/Download
- Cut/Copy/Paste
- Rename
- Delete (to trash or permanent)
- Compress selected items
- Properties view

### Storage Management
- Multiple mount point configuration
- Storage target selection (PVC, StorageClass, HostPath)
- Real-time storage statistics

## API Endpoints

### File Operations

#### GET /api/list?path={path}
Lists directory contents with metadata.

**Query Parameters:**
- `path` (default: "/"): Directory path
- `hidden` (default: false): Show hidden files
- `sort` (default: "name"): Sort by name, size, modified, or type
- `order` (default: "asc"): Sort order

**Response:**
```json
{
  "path": "/documents",
  "parent": "/",
  "breadcrumbs": [{"name": "Home", "path": "/"}, {"name": "documents", "path": "/documents"}],
  "items": [
    {
      "name": "file.txt",
      "path": "/documents/file.txt",
      "isDir": false,
      "size": 1024,
      "modified": "2026-01-17T12:00:00",
      "mimeType": "text/plain",
      "icon": "file-text"
    }
  ],
  "count": 1
}
```

#### GET /api/file?path={path}&download={bool}
Downloads or serves a file.

#### POST /api/upload
Uploads files to a directory.

**Form Data:**
- `path`: Target directory
- `file`: File(s) to upload

#### POST /api/mkdir
Creates a new directory.

**Request Body:**
```json
{
  "path": "/documents",
  "name": "New Folder"
}
```

#### POST /api/move
Moves a file or directory.

**Request Body:**
```json
{
  "source": "/documents/old.txt",
  "destination": "/archive/old.txt"
}
```

#### POST /api/copy
Copies a file or directory.

#### POST /api/rename
Renames a file or directory.

**Request Body:**
```json
{
  "path": "/documents/old.txt",
  "name": "new.txt"
}
```

#### DELETE /api/delete?path={path}&permanent={bool}
Deletes a file or moves to trash.

#### POST /api/delete-multiple
Deletes multiple items at once.

**Request Body:**
```json
{
  "paths": ["/file1.txt", "/file2.txt"],
  "permanent": false
}
```

### Search

#### GET /api/search?q={query}&path={path}&type={type}&limit={n}
Searches for files by name.

**Query Parameters:**
- `q`: Search query (required)
- `path`: Starting directory (default: "/")
- `type`: Filter by type (folder, image, document, video, audio)
- `limit`: Max results (default: 100)

### Recent Files & Bookmarks

#### GET /api/recent
Returns recently accessed files.

#### GET /api/locations
Returns default navigation locations.

#### GET/POST/DELETE /api/bookmarks
Manages user bookmarks.

### Trash Management

#### GET /api/trash
Lists items in trash.

#### POST /api/trash/restore
Restores an item from trash.

**Request Body:**
```json
{
  "path": "/.trash/123456_file.txt"
}
```

#### DELETE /api/trash/empty
Empties the trash permanently.

### Preview & Thumbnails

#### GET /api/preview?path={path}
Returns file preview data.

#### GET /api/thumbnail?path={path}&size={pixels}
Returns image thumbnail (cached).

#### GET /api/info?path={path}
Returns detailed file/directory information.

### Archives

#### POST /api/compress
Creates a ZIP archive.

**Request Body:**
```json
{
  "paths": ["/file1.txt", "/folder1"],
  "name": "archive.zip",
  "destination": "/"
}
```

#### POST /api/decompress
Extracts a ZIP archive.

### Storage Configuration

#### GET /api/stats
Returns storage statistics.

#### GET /api/storage/config
Returns storage configuration.

#### POST /api/storage/config
Updates storage configuration.

#### GET /api/storage/mounts
Lists available mount points with accessibility status.

#### POST /api/storage/mounts
Adds a new mount path.

#### DELETE /api/storage/mounts?path={path}
Removes a mount path.

#### POST /api/storage/switch
Switches to a different mount path as the base.

### Service Status

#### GET /api/services
Checks health of related microservices.

#### GET /health
Health check endpoint.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FILE_BASE_PATH` | /data | Base path for file operations |
| `FILE_LIST_URL` | http://file-list.holm.svc.cluster.local:8080 | File list microservice |
| `FILE_UPLOAD_URL` | http://file-upload.holm.svc.cluster.local:8080 | Upload microservice |
| (and more for each microservice) | | |

## Default Storage Configuration

```json
{
  "basePath": "/data",
  "storageTarget": {
    "type": "pvc",
    "name": "files-pvc",
    "storageClass": "local-path",
    "hostPath": "/mnt/node13-ssd/files",
    "node": "node13"
  },
  "mountPaths": [
    {"name": "Primary (node13 SSDs)", "path": "/mnt/node13-ssd/files", "node": "node13", "default": true}
  ]
}
```

## Icon Mapping

Files are assigned icons based on extension or MIME type:
- Documents: .pdf, .doc, .docx, .txt, .md
- Code: .py, .js, .ts, .html, .css, .json
- Media: .mp3, .mp4, .jpg, .png, .gif
- Archives: .zip, .tar, .gz, .rar

Directories have special icons for common names (Documents, Downloads, Music, etc.)

## Screenshot Description

The File Web Nautilus interface presents a familiar file manager layout with a sidebar on the left showing quick access locations (Home, Documents, Downloads, etc.), bookmarks, and storage statistics. The main area displays files in a grid or list view with icons representing file types. A breadcrumb bar at the top shows the current path. Files show name, size, and modification date. Selected files highlight with a blue accent, and right-click reveals a context menu for operations.
