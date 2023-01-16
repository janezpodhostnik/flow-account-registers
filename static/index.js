let app = {}
app.ws = undefined
app.container = undefined
app.caret = undefined

app.print = function (message) {
    app.container.innerHTML += "\n" + message
    app.caret.scrollIntoView()
}

app.print = function (message) {
    app.container.innerHTML += "\n" + message
    app.caret.scrollIntoView()
}

app.printBlockHeight = function (message) {
    app.title.innerHTML += " " + message
}


app.init = function () {
    if (!(window.WebSocket)) {
        alert('Your browser does not support WebSocket')
        return
    }

    app.title = document.querySelector('.title')
    app.container = document.querySelector('.container')
    app.caret = document.querySelector('.caret')

    app.address = app.title.getAttribute("address")

    if (!app.address) {
        app.print("An app error occurred. Address is expected to not be empty at this point.")
        return
    }

    app.ws = new WebSocket('ws://localhost:8080/ws?address=' + app.address)

    app.ws.onmessage = function (event) {
        let e = JSON.parse(event.data)
        switch (e.type) {
            case 'register':
                app.print(e.register_header)
                app.print(e.register)
                break
            case 'storage':
                app.print(e.storage_message)
                break
            case 'block':
                app.printBlockHeight(e.block_height)
                break
            case 'error':
                app.print(e.error)
                break
        }
    }

    app.ws.onclose = function () {
        let message = 'done'
        app.caret.remove()
        app.print(message)
    }
}

window.onload = app.init