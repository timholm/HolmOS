import sys

# Read HTML
with open("/home/rpi1/builds/settings-web/ui.html", "r") as f:
    html = f.read()

# Read Go template
with open("/home/rpi1/builds/settings-web/main.go", "r") as f:
    go_code = f.read()

# Escape backticks in HTML for Go raw string
html_escaped = html.replace("`", "` + \"`\" + `")

# Replace placeholder
go_code = go_code.replace("HTMLPLACEHOLDER", html_escaped)

# Write final Go file
with open("/home/rpi1/builds/settings-web/main.go", "w") as f:
    f.write(go_code)

print("Combined successfully")
