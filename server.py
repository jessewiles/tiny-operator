#!/usr/bin/env python
# Echo the current environment

from BaseHTTPServer import HTTPServer, BaseHTTPRequestHandler
import json
import os

class RequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200, 'OK')
        self.send_header("Content-type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(dict(os.environ), sort_keys=True, indent=4, separators=(',', ': ')))
        self.wfile.flush()
        self.wfile.close()

    do_POST = do_GET
    do_PUT = do_POST
    do_DELETE = do_GET

def main():
    port = 8000
    print('Listening on localhost:%s' % port)
    server = HTTPServer(('', port), RequestHandler)
    server.serve_forever()

if __name__ == "__main__":
    main()
