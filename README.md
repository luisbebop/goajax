Go Ajax
=======

  Go Ajax is a JSON-RPC implementation that is designed to work with AJAX.  Go Ajax is suitable for use with JQuery and can be used to build rich AJAX web applications in Google Go.

Install
-------
  To build and install the library run the following commands:
  
    export GOPATH=/path/to/install
    git clone https://github.com/jeffreybolle/goajax.git
    cd goajax
    cp -R goajax $GOPATH/src
    go install goajax

Example
-------
  To build and run the example run the following commands:
    
    go run example.go
    

  This will start the example web service listening on port 9001.  Start any browser and browse to <a href="http://localhost:9001/">http://localhost:9001/</a>.

LICENSE:
--------

  The library is available under the same terms and conditions as the Go, the BSD style license, and the LGPL (Lesser GNU Public License).

AUTHORS:
--------

 * Jeffrey Bolle
