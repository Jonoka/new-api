{
  "id": "{{module_id}}",
  "name": "{{module_name}}",
  "version": "0.1.0",
  "description": "{{module_description}}",
  "runtime": {
    "type": "http",
    "base_url": "http://127.0.0.1:{{port}}",
    "health_path": "/health"
  },
  "ui": {
    "nav": [
      {
        "title": "{{nav_title}}",
        "page": "index",
        "icon": "Puzzle",
        "section": "admin",
        "order": 100
      }
    ],
    "pages": [
      {
        "key": "index",
        "title": "{{nav_title}}",
        "path": "/ui",
        "embed": true
      }
    ]
  },
  "permissions": {
    "roles": ["root"]
  }
}
