# partial.js >> The X-Partial Clientside Library

`This library is part of the x-partial project. X-Partial embraces the hypermedia concept.`

partial.js is a lightweight JavaScript library designed to simplify AJAX interactions and dynamic content updates in web applications. It enables developers to enhance their web pages with partial page updates, form submissions, custom event handling, and browser history management without the need for full-page reloads.

## Installation
Include the partial.js script in your HTML file:

```html
<script src="partial.js"></script>
```

## Getting Started
Instantiate the Partial class in your main JavaScript file:

```javascript
const xp = new Partial({
    // Configuration options (optional)
});
```
This will automatically scan the document for elements with x-* attributes and set up the necessary event listeners.

## Attributes
partial.js leverages custom HTML attributes to define actions and behaviors:

- Action Attributes:
  - **x-get**: Defines a GET request.
  - **x-post**: Defines a POST request.
  - **x-put**: Defines a PUT request.
  - **x-delete**: Defines a DELETE request.
- Targeting:
  - **x-target**: Specifies the selector of the element where the response content will be injected.
- Trigger Events:
    - **x-trigger**: Defines the event that triggers the action (e.g., click, submit). Defaults to click for most elements and submit for forms.
- Serialization:
    - **x-serialize**: When set to json, form data will be serialized as JSON.
- Custom Headers and Data:
  - **x-json**: Provides a JSON string to be sent as the request body.
  - **x-params**: Provides JSON parameters to be appended to the URL for GET requests.
- Out-of-Band Swapping:
  - **x-swap-oob**: Indicates elements that should be swapped out-of-band (e.g., updating elements outside the main content area).
- Browser History:
    - **x-push-state**: When set to 'false', disables updating the browser history. Defaults to updating history.
- Focus Management:
    - **x-focus**: When set to 'false', disables auto-focus on the target element after content update.
- Debouncing:
    - **x-debounce**: Specifies the debounce time in milliseconds for the event handler.
- Before and after hooks:
    - **x-before**: Specifies an event to be triggered before the request is sent.
    - **x-after**: Specifies an event to be triggered after the response is received.

## Configuration Options
When instantiating partial.js, you can provide a configuration object to customize its behavior:

```javascript
const xp = new Partial({
    onError: (error, element) => {
        // Custom error handling
    },
    csrfToken: 'your-csrf-token' || (() => /* return token */),
    beforeRequest: async ({ method, url, headers, element }) => {
        // Logic before the request is sent
    },
    afterResponse: async ({ response, element }) => {
        // Logic after the response is received
    },
    autoFocus: true, // Automatically focus the target element (default: true)
    debounceTime: 200, // Debounce event handlers by 200 milliseconds (default: 0)
    defaultSwapOption: 'innerHTML', // Default content swap method ('outerHTML' or 'innerHTML')
});
```

### Available Options:

- `onError` (Function): Callback function to handle errors. Receives error and element as arguments.
- `csrfToken` (Function or string): CSRF token value or a function that returns the token. Automatically included in request headers as X-CSRF-Token.
- `beforeRequest` (Function): Hook function called before a request is sent. Receives an object with method, url, headers, and element.
- `afterResponse` (Function): Hook function called after a response is received. Receives an object with response and element.
- `autoFocus` (boolean): Whether to auto-focus the target element after content update. Default is true.
- `debounceTime` (number): Debounce time in milliseconds for event handlers. Default is 0 (no debounce).
- `defaultSwapOption` ('outerHTML' | 'innerHTML'): Default swap method for content replacement. Default is 'outerHTML'.

## Class Overview

### Partial

The main class that handles scanning the DOM, setting up event listeners, making AJAX requests, updating the DOM based on responses, and managing browser history.

#### Parameters:

- `options` (Object): Configuration options (see Configuration Options).

#### Description:

Initializes the Partial instance, sets up action attributes, binds methods, and sets up event listeners. It automatically scans the document for elements with action attributes on DOMContentLoaded and listens for popstate events for browser navigation.

### event
```javascript
xp.event(eventName, callback, options)
```

#### Parameters:
- `eventName` (string): The name of the event to listen for.
- `callback` (Function): The function to call when the event is dispatched.
- `options` (boolean | AddEventListenerOptions): Optional event listener options.

#### Description:
Registers an event listener for a custom event dispatched by Partial.

### removeEvent
```javascript
xp.event(eventName, callback, options)
```

#### Parameters:
- `eventName` (string): The name of the event to listen for.
- `callback` (Function): The function to call when the event is dispatched.
- `options` (boolean | AddEventListenerOptions): Optional event listener options.

#### Description:
Registers an event listener for a custom event dispatched by Partial.

### removeAllEvents

#### Parameters:
- `eventName` (string): The name of the event.

#### Description:
Removes all event listeners registered for the specified event name.

### refresh
```javascript
xp.refresh(container = document)
```

#### Parameters:
- `container` (HTMLElement): The container element to scan for x-* attributes. Defaults to the entire document.

#### Description:
Manually re-scans a specific container for Partial elements. Useful when dynamically adding content to the DOM.

## Usage Examples 

### Basic Link Click 
```html
<!-- HTML -->
<a href="/new-content" x-get="/new-content" x-target="#content">Load Content</a>

<div id="content">
  <!-- Dynamic content will be loaded here -->
</div>
```

#### Description:
When the link is clicked, Partial intercepts the click event, performs a GET request to /new-content, and injects the response into the element with ID content. The browser history is updated accordingly.

### Form Submission
```html
<!-- HTML -->
<form x-post="/submit" x-target="#content" x-serialize="json">
  <input type="text" name="username" />
  <button type="submit">Submit</button>
</form>

<div id="content">
  <!-- Response content will be loaded here -->
</div>
```

#### Description:
Upon form submission, Partial sends a POST request to /submit, serializes the form data as JSON, and updates the #content element with the response. The default form submission is prevented.

### Handling Custom Events
```javascript
// JavaScript
xp.event('notify', (event) => {
  alert(event.detail.message);
});
```

#### Server Response Headers:
```css
X-Event-Notify: {"message": "Operation successful"}
```

#### Description:
When the server responds with an X-Event-Notify header, Partial dispatches a notify event. The registered event listener displays an alert with the message.

### Out-of-Band (OOB) Swapping
```html 
<!-- HTML -->
<!-- Button to trigger the action -->
<button x-get="/update" x-target="#content">Update</button>

<!-- Element to be updated out-of-band -->
<div id="status">
  Current status: Active
</div>
```
#### Server Response:
```html 
<!-- Partial content -->
<div id="content">
  <!-- Main content updates here -->
</div>

<!-- OOB element -->
<div id="status" x-swap-oob>
  Current status: Inactive
</div>
```

#### Description:
The OOB element with x-swap-oob is processed by Partial to update the #status element even though it's outside the main #content area.

### Browser History Management
```html 
<!-- HTML -->
<a href="/page2" x-get="/page2" x-target="#content">Go to Page 2</a>

<div id="content">
  <!-- Content updates here -->
</div>
```

#### Description:
When the link is clicked, Partial updates the content and uses history.pushState to update the browser's URL. The popstate event handler ensures that navigating back and forward works correctly by reloading content based on the current URL.


## Advanced Features
### Custom Headers
Add custom headers by using x-* attributes. For example:
```html 
<button x-get="/data" x-custom-header="value" x-target="#content">Load Data</button>
```
#### Description:
This will send a request to /data with a header X-Custom-Header: value.


### Event Lifecycle Hooks
Partial provides hooks to execute custom logic before and after requests:

- beforeRequest Hook:
  ```javascript
  const xp = new Partial({
    beforeRequest: async ({ method, url, headers, element }) => {
    // Logic before the request is sent
    },
  });
  ```
- afterResponse Hook:
  ```javascript
  const xp = new Partial({
    afterResponse: async ({ response, element }) => {
    // Logic after the response is received
    },
  });
  ```

### Debounce Functionality
Prevent rapid, repeated triggering of event handlers:
```javascript
const xp = new Partial({
    debounceTime: 300, // Debounce by 300 milliseconds
});
```
#### Description:
This is particularly useful for events like input or rapid clicks, ensuring the event handler is not called more often than the specified debounce time.

### Focus Management
Control whether the target element receives focus after content is updated:

- Globally Enable Auto-Focus:
  ```javascript
  const xp = new Partial({
      autoFocus: true,
  });
  ```
- Disable Auto-Focus on Specific Elements:
  ```html
  <button x-get="/data" x-target="#content" x-focus="false">Load Data</button>
  ```
    

## Contributing
Contributions are welcome! Please submit issues and pull requests on the GitHub repository.

## License
This project is licensed under the MIT License.