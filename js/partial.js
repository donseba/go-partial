// @ts-check

/**
 * @typedef {Object} PartialOptions
 * @property {'outerHTML'|'innerHTML'} [defaultSwapOption='outerHTML'] - Default swap method.
 * @property {Function} [onError] - Callback function for handling errors.
 * @property {Function|string} [csrfToken] - CSRF token value or function returning the token.
 * @property {Function} [beforeRequest] - Hook before the request is sent.
 * @property {Function} [afterResponse] - Hook after the response is received.
 * @property {boolean} [autoFocus=true] - Whether to auto-focus the target element after content update.
 * @property {number} [debounceTime=0] - Debounce time in milliseconds for event handlers.
 */

/**
 * @typedef {Object} SseMessage
 * @property {string} content - The HTML content to insert.
 * @property {string} [xTarget] - The CSS selector for the target element.
 * @property {string} [xFocus] - Whether to focus the target element ('true' or 'false').
 * @property {string} [xSwap] - The swap method ('outerHTML' or 'innerHTML').
 * @property {string} [xEvent] - Custom events to dispatch.
 */

/**
 * Class representing Partial.js.
 */
class Partial {
    /**
     * Creates an instance of Partial.
     * @param {PartialOptions} [options={}] - Configuration options.
     */
    constructor(options = {}) {
        // Define the custom action attributes
        this.ATTRIBUTES = {
            ACTIONS: {
                GET:    'x-get',
                POST:   'x-post',
                PUT:    'x-put',
                DELETE: 'x-delete',
            },
            TARGET:     'x-target',
            TRIGGER:    'x-trigger',
            SERIALIZE:  'x-serialize',
            JSON:       'x-json',
            PARAMS:     'x-params',
            SWAP_OOB:   'x-swap-oob',
            PUSH_STATE: 'x-push-state',
            FOCUS:      'x-focus',
            DEBOUNCE:   'x-debounce',
            BEFORE:     'x-before',
            AFTER:      'x-after',
            SSE:        'x-sse',
        };

        // Store options with default values
        this.onError = options.onError || null;
        this.csrfToken = options.csrfToken || null;
        this.defaultSwapOption = options.defaultSwapOption || 'outerHTML';
        this.beforeRequest = options.beforeRequest || null;
        this.afterResponse = options.afterResponse || null;
        this.autoFocus = options.autoFocus !== undefined ? options.autoFocus : false;
        this.debounceTime = options.debounceTime || 0;

        this.eventTarget = new EventTarget();
        this.eventListeners = {};

        // Map to store SSE connections per element
        this.sseConnections = new Map();

        // Bind methods to ensure correct 'this' context
        this.scanForElements = this.scanForElements.bind(this);
        this.setupElement = this.setupElement.bind(this);
        this.setupSSEElement = this.setupSSEElement.bind(this);
        this.handleAction = this.handleAction.bind(this);
        this.handleOobSwapping = this.handleOobSwapping.bind(this);
        this.handlePopState = this.handlePopState.bind(this);

        // Initialize the handler on DOMContentLoaded
        document.addEventListener('DOMContentLoaded', () => this.scanForElements());

        // Listen for popstate events
        window.addEventListener('popstate', this.handlePopState);
    }

    /**
     * Scans the entire document or a specific container for elements with defined action attributes.
     * @param {HTMLElement | Document} [container=document]
     */
    scanForElements(container = document) {
        const actionSelector = Object.values(this.ATTRIBUTES.ACTIONS).map(attr => `[${attr}]`).join(',');
        const sseSelector = `[${this.ATTRIBUTES.SSE}]`;
        const combinedSelector = `${actionSelector}, ${sseSelector}`;
        const elements = container.querySelectorAll(combinedSelector);

        elements.forEach(element => {
            if (element.hasAttribute(this.ATTRIBUTES.SSE)) {
                this.setupSSEElement(element);
            } else {
                this.setupElement(element);
            }
        });
    }

    /**
     * Sets up an element with x-sse attribute to handle SSE connections.
     * @param {HTMLElement} element
     */
    setupSSEElement(element) {
        // Avoid attaching multiple listeners
        if (element.__xSSEInitialized) return;

        const sseUrl = element.getAttribute(this.ATTRIBUTES.SSE);
        if (!sseUrl) {
            console.error('No URL specified in x-sse attribute on element:', element);
            return;
        }

        const eventSource = new EventSource(sseUrl);

        eventSource.onmessage = (event) => {
            this.handleSSEMessage(event, element);
        };

        eventSource.onerror = (error) => {
            console.error('SSE connection error on element:', element, error);
            if (typeof this.onError === 'function') {
                this.onError(error, element);
            }
        };

        // Store the connection to manage it later if needed
        this.sseConnections.set(element, eventSource);

        // Mark the element as initialized
        element.__xSSEInitialized = true;
    }

    /**
     * Handles incoming SSE messages for a specific element.
     * @param {MessageEvent} event
     * @param {HTMLElement} element
     */
    async handleSSEMessage(event, element) {
        try {
            /** @type {SseMessage} */
            const data = JSON.parse(event.data);

            const targetSelector = data.xTarget;
            const targetElement = document.querySelector(targetSelector);

            if (!targetElement) {
                console.error(`No element found with selector '${targetSelector}' for SSE message.`);
                return;
            }

            // Decide swap method
            const swapOption = data.xSwap || this.defaultSwapOption;

            if (swapOption === 'outerHTML') {
                targetElement.outerHTML = data.content;
            } else if (swapOption === 'innerHTML') {
                targetElement.innerHTML = data.content;
            } else {
                console.error(`Invalid x-swap option '${swapOption}' in SSE message. Use 'outerHTML' or 'innerHTML'.`);
                return;
            }

            // Optionally focus the target element
            const focusEnabled = data.xFocus !== 'false';
            if (this.autoFocus && focusEnabled) {
                const newTargetElement = document.querySelector(targetSelector);
                if (newTargetElement) {
                    if (newTargetElement.getAttribute('tabindex') === null) {
                        newTargetElement.setAttribute('tabindex', '-1');
                    }
                    newTargetElement.focus();
                }
            }

            // Re-scan the updated content for Partial elements
            this.scanForElements();

            // Dispatch custom events if specified
            if (data.xEvent) {
                await this.dispatchCustomEvents(data.xEvent, { element, event, data });
            }

            // Dispatch an event after the content is replaced
            this.dispatchEvent('sseContentReplaced', { targetElement, data, element });

        } catch (error) {
            console.error('Error processing SSE message:', error);
            if (typeof this.onError === 'function') {
                this.onError(error, element);
            }
        }
    }

    /**
     * Sets up an individual element by attaching the appropriate event listener.
     * @param {HTMLElement} element
     */
    setupElement(element) {
        // Avoid attaching multiple listeners
        if (element.__xRequestHandlerInitialized) return;

        // Set a default trigger based on the element type
        let trigger;
        if (element.tagName === 'FORM') {
            trigger = element.getAttribute(this.ATTRIBUTES.TRIGGER) || 'submit';
        } else {
            trigger = element.getAttribute(this.ATTRIBUTES.TRIGGER) || 'click';
        }

        // Get custom debounce time from x-debounce attribute
        let elementDebounceTime = this.debounceTime; // Default to global debounce time
        const xDebounce = element.getAttribute(this.ATTRIBUTES.DEBOUNCE);
        if (xDebounce !== null) {
            const parsedDebounce = parseInt(xDebounce, 10);
            if (!isNaN(parsedDebounce) && parsedDebounce >= 0) {
                elementDebounceTime = parsedDebounce;
            } else {
                console.warn(`Invalid x-debounce value '${xDebounce}' on element:`, element);
            }
        }

        // Debounce only the handleAction function
        const debouncedHandleAction = this.debounce((event) => {
            this.handleAction(event, element);
        }, elementDebounceTime);

        // Event handler that calls preventDefault immediately
        const handler = (event) => {
            event.preventDefault();
            debouncedHandleAction(event);
        };

        element.addEventListener(trigger, handler);

        // Mark the element as initialized
        element.__xRequestHandlerInitialized = true;
    }

    /**
     * Handles the action when an element is triggered.
     * @param {Event} event
     * @param {HTMLElement} element
     */
    async handleAction(event, element) {
        const requestParams = this.extractRequestParams(element);

        // Ensure 'element' is included in the request parameters
        requestParams.element = element;

        if (!requestParams.url) {
            const error = new Error(`No URL specified for method ${requestParams.method} on element.`);
            console.error(error.message, element);
            if (typeof this.onError === 'function') {
                this.onError(error, element);
            }
            return;
        }

        const targetElement = document.querySelector(requestParams.targetSelector);
        if (!targetElement) {
            const error = new Error(`No element found with selector '${requestParams.targetSelector}' for 'x-target' targeting.`);
            console.error(error.message);
            if (typeof this.onError === 'function') {
                this.onError(error, element);
            }
            return;
        }

        if (!requestParams.partialId) {
            const error = new Error(`Target element does not have an 'id' attribute.`);
            console.error(error.message, targetElement);
            if (typeof this.onError === 'function') {
                this.onError(error, element);
            }
            return;
        }

        // Set the X-Target header to the request
        requestParams.headers["X-Target"] = requestParams.partialId;

        try {
            // Dispatch x-before event(s) if specified
            const beforeEvents = element.getAttribute(this.ATTRIBUTES.BEFORE);
            if (beforeEvents) {
                await this.dispatchCustomEvents(beforeEvents, { element, event });
            }

            // Before request hook
            if (typeof this.beforeRequest === 'function') {
                await this.beforeRequest({ ...requestParams, element });
            }

            // Dispatch beforeSend event
            this.dispatchEvent('beforeSend', { ...requestParams, element });

            const responseText = await this.performRequest(requestParams);

            // After response hook
            if (typeof this.afterResponse === 'function') {
                await this.afterResponse({ response: this.lastResponse, element });
            }

            // Dispatch afterReceive event
            this.dispatchEvent('afterReceive', { response: this.lastResponse, element });

            // Process and update the DOM with the response
            await this.processResponse(responseText, targetElement, element);

            // After successfully updating content
            const shouldPushState = element.getAttribute(this.ATTRIBUTES.PUSH_STATE) !== 'false';
            if (shouldPushState) {
                const newUrl = new URL(requestParams.url, window.location.origin);
                history.pushState({ xPartial: true }, '', newUrl);
            }

            // Dispatch x-after event(s) if specified
            const afterEvents = element.getAttribute(this.ATTRIBUTES.AFTER);
            if (afterEvents) {
                await this.dispatchCustomEvents(afterEvents, { element, event });
            }
        } catch (error) {
            console.error('Request failed:', error);

            if (typeof this.onError === 'function') {
                this.onError(error, element);
            } else {
                // Optionally, handle error display in the UI
                targetElement.innerHTML = `<div class="error">An error occurred: ${error.message}</div>`;
            }
        }
    }

    /**
     * Extracts request parameters from the element.
     * @param {HTMLElement} element
     * @returns {Object} Parameters including method, url, headers, body, etc.
     */
    extractRequestParams(element) {
        const method = this.getMethod(element);
        let url = element.getAttribute(`x-${method.toLowerCase()}`);

        const headers = this.getHeaders(element);

        let targetSelector = element.getAttribute(this.ATTRIBUTES.TARGET);
        if (!targetSelector) {
            targetSelector = 'body';
        }

        const targetElement = document.querySelector(targetSelector);
        const partialId = targetElement ? targetElement.getAttribute('id') : null;

        // Handle x-params for GET requests
        if (method === 'GET') {
            const xParams = element.getAttribute(this.ATTRIBUTES.PARAMS);
            if (xParams) {
                try {
                    const paramsObject = JSON.parse(xParams);
                    const urlParams = new URLSearchParams(paramsObject).toString();
                    url += (url.includes('?') ? '&' : '?') + urlParams;
                } catch (e) {
                    console.error('Invalid JSON in x-params attribute:', e);
                    const error = new Error('Invalid JSON in x-params attribute');
                    if (typeof this.onError === 'function') {
                        this.onError(error, element);
                    }
                }
            }
        }

        return { method, url, headers, targetSelector, partialId };
    }

    /**
     * Determines the HTTP method based on the element's attributes.
     * @param {HTMLElement} element
     * @returns {string} HTTP method
     */
    getMethod(element) {
        for (const attr of Object.values(this.ATTRIBUTES.ACTIONS)) {
            if (element.hasAttribute(attr)) {
                return attr.replace('x-', '').toUpperCase();
            }
        }
        return 'GET'; // Default method
    }

    /**
     * Constructs headers from the element's attributes.
     * @param {HTMLElement} element
     * @returns {Object} Headers object
     */
    getHeaders(element) {
        const headers = {};

        if (this.csrfToken) {
            if (typeof this.csrfToken === 'function') {
                headers['X-CSRF-Token'] = this.csrfToken();
            } else {
                headers['X-CSRF-Token'] = this.csrfToken;
            }
        }

        // Collect all x-* attributes that are not actionAttributes
        for (const attr of element.attributes) {
            const name = attr.name;
            if (name.startsWith('x-') && !Object.values(this.ATTRIBUTES.ACTIONS).includes(name)) {
                const headerName = 'X-' + this.capitalize(name.substring(2)); // Remove 'x-' prefix and capitalize
                headers[headerName] = attr.value;
            }
        }

        return headers;
    }

    /**
     * Capitalizes the first letter of the string.
     * @param {string} str
     * @returns {string}
     */
    capitalize(str) {
        return str.charAt(0).toUpperCase() + str.slice(1);
    }

    /**
     * Performs the HTTP request using Fetch API.
     * @param {Object} requestParams - Parameters including method, url, headers, body, etc.
     * @returns {Promise<string>} Response text
     */
    performRequest(requestParams) {
        const { method, url, headers, element } = requestParams;
        const options = {
            method,
            headers,
            credentials: 'same-origin',
        };

        // Handle request body
        if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
            let body = null;

            // Check if the element or the closest form has x-serialize="json"
            const serializeAsJson = element && (element.getAttribute(this.ATTRIBUTES.SERIALIZE) === 'json' ||
                (element.closest('form') && element.closest('form').getAttribute(this.ATTRIBUTES.SERIALIZE) === 'json'));

            // Check for x-json attribute
            const xJson = element && element.getAttribute(this.ATTRIBUTES.JSON);

            if (xJson) {
                // Parse x-json attribute
                try {
                    body = JSON.stringify(JSON.parse(xJson));
                    headers['Content-Type'] = 'application/json';
                } catch (e) {
                    console.error('Invalid JSON in x-json attribute:', e);
                    throw new Error('Invalid JSON in x-json attribute');
                }
            } else if (element && (element.tagName === 'FORM' || element.closest('form'))) {
                const form = element.tagName === 'FORM' ? element : element.closest('form');
                if (serializeAsJson) {
                    // Serialize form data as JSON
                    const formData = new FormData(form);
                    const jsonObject = {};
                    formData.forEach((value, key) => {
                        // Handle multiple values per key (e.g., checkboxes)
                        if (jsonObject[key]) {
                            if (Array.isArray(jsonObject[key])) {
                                jsonObject[key].push(value);
                            } else {
                                jsonObject[key] = [jsonObject[key], value];
                            }
                        } else {
                            jsonObject[key] = value;
                        }
                    });
                    body = JSON.stringify(jsonObject);
                    headers['Content-Type'] = 'application/json';
                } else {
                    // Use FormData
                    body = new FormData(form);
                }
            }

            if (body) {
                options.body = body;
            }
        }

        return fetch(url, options).then(response => {
            // Store the response for event handling
            this.lastResponse = response;

            if (!response.ok) {
                return response.text().then(text => {
                    throw new Error(`HTTP error ${response.status}: ${text}`);
                });
            }
            return response.text();
        });
    }

    /**
     * Processes the response text and updates the DOM accordingly.
     * @param {string} responseText
     * @param {HTMLElement} targetElement
     * @param {HTMLElement} element
     */
    async processResponse(responseText, targetElement, element) {
        // Dispatch beforeUpdate event
        this.dispatchEvent('beforeUpdate', { targetElement, element });

        // Parse the response HTML
        const parser = new DOMParser();
        const doc = parser.parseFromString(responseText, 'text/html');

        // Extract OOB elements
        const oobElements = Array.from(doc.querySelectorAll(`[${this.ATTRIBUTES.SWAP_OOB}]`));
        oobElements.forEach(el => el.parentNode.removeChild(el));

        // Replace the target's content
        this.updateTargetElement(targetElement, doc);

        // Dispatch afterUpdate event
        this.dispatchEvent('afterUpdate', { targetElement, element });

        // Re-scan the newly added content for Partial elements
        this.scanForElements(targetElement);

        // Handle OOB swapping with the extracted OOB elements
        this.handleOobSwapping(oobElements);

        // Handle any x-event-* headers from the response
        await this.handleResponseEvents();

        // Auto-focus if enabled
        const focusEnabled = element.getAttribute(this.ATTRIBUTES.FOCUS) !== 'false';
        if (this.autoFocus && focusEnabled) {
            if (targetElement.getAttribute('tabindex') === null) {
                targetElement.setAttribute('tabindex', '-1');
            }
            targetElement.focus();
        }
    }

    /**
     * Updates the target element with new content.
     * @param {HTMLElement} targetElement
     * @param {Document} doc
     */
    updateTargetElement(targetElement, doc) {
        targetElement.innerHTML = doc.body.innerHTML;
    }

    /**
     * Handles Out-of-Band (OOB) swapping by processing an array of OOB elements.
     * Replaces existing elements in the document with the new content based on matching IDs.
     * @param {HTMLElement[]} oobElements
     */
    handleOobSwapping(oobElements) {
        oobElements.forEach(oobElement => {
            const targetId = oobElement.getAttribute('id');
            if (!targetId) {
                console.error('OOB element does not have an ID:', oobElement);
                return;
            }

            const swapOption = oobElement.getAttribute(this.ATTRIBUTES.SWAP_OOB) || this.defaultSwapOption;
            const existingElement = document.getElementById(targetId);

            if (!existingElement) {
                console.error(`No existing element found with ID '${targetId}' for OOB swapping.`);
                return;
            }

            if (swapOption === 'outerHTML' || swapOption === true) {
                existingElement.outerHTML = oobElement.outerHTML;
            } else if (swapOption === 'innerHTML') {
                existingElement.innerHTML = oobElement.innerHTML;
            } else {
                console.error(`Invalid x-swap-oob option '${swapOption}' on element with ID '${targetId}'. Use 'outerHTML' or 'innerHTML'.`);
                return;
            }

            // After swapping, initialize any new elements within the replaced content
            const newElement = document.getElementById(targetId);
            if (newElement) {
                this.scanForElements(newElement);
            }
        });
    }

    /**
     * Handles any x-event-* headers from the response and dispatches events accordingly.
     */
    async handleResponseEvents() {
        if (!this.lastResponse || !this.lastResponse.headers) {
            return;
        }

        this.lastResponse.headers.forEach((value, name) => {
            if (name.toLowerCase().startsWith('x-event-')) {
                const eventName = name.substring(8); // Remove 'x-event-' prefix
                let eventData = value;
                try {
                    eventData = JSON.parse(value);
                } catch (e) {
                    // Value is not JSON, use as is
                }
                this.dispatchEvent(eventName, eventData);
            }
        });
    }

    /**
     * Dispatches custom events specified in a comma-separated string.
     * @param {string} events - Comma-separated event names.
     * @param {Object} detail - Detail object to pass with the event.
     */
    async dispatchCustomEvents(events, detail) {
        const eventNames = events.split(',').map(e => e.trim());
        for (const eventName of eventNames) {
            const event = new CustomEvent(eventName, { detail });
            this.eventTarget.dispatchEvent(event);
        }
    }

    /**
     * Handles the popstate event for browser navigation.
     * @param {PopStateEvent} event
     */
    async handlePopState(event) {
        if (event.state && event.state.xPartial) {
            const url = window.location.href;
            try {
                const responseText = await this.performRequest({ method: 'GET', url, headers: {}, element: null });

                // Parse the response HTML
                const parser = new DOMParser();
                const doc = parser.parseFromString(responseText, 'text/html');

                // Replace the body content
                document.body.innerHTML = doc.body.innerHTML;

                // Re-scan the entire document
                this.scanForElements();

                // Optionally, focus the body
                if (this.autoFocus) {
                    document.body.focus();
                }

            } catch (error) {
                console.error('PopState request failed:', error);
                if (typeof this.onError === 'function') {
                    this.onError(error, document.body);
                }
            }
        }
    }

    /**
     * Debounce function to limit the rate at which a function can fire.
     * @param {Function} func - The function to debounce.
     * @param {number} wait - The number of milliseconds to wait.
     * @returns {Function}
     */
    debounce(func, wait) {
        let timeout;
        return (...args) => {
            const later = () => {
                clearTimeout(timeout);
                func.apply(this, args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    /**
     * Listens for a custom event and executes the callback when the event is dispatched.
     * @param {string} eventName - The name of the event to listen for
     * @param {Function} callback - The function to call when the event is dispatched
     * @param {boolean | AddEventListenerOptions} [options] - Optional options for addEventListener.
     */
    event(eventName, callback, options) {
        if (!this.eventListeners[eventName]) {
            this.eventListeners[eventName] = [];
        }
        this.eventListeners[eventName].push({ callback, options });
        this.eventTarget.addEventListener(eventName, callback, options);
    }

    /**
     * Removes a custom event listener.
     * @param {string} eventName - The name of the event to remove
     * @param {Function} callback - The function to remove
     * @param {boolean | AddEventListenerOptions} [options] - Optional options for addEventListener.
     */
    removeEvent(eventName, callback, options) {
        if (this.eventListeners[eventName]) {
            // Find the index of the listener to remove
            const index = this.eventListeners[eventName].findIndex(
                (listener) => listener.callback === callback && JSON.stringify(listener.options) === JSON.stringify(options)
            );
            if (index !== -1) {
                // Remove the listener from the registry
                this.eventListeners[eventName].splice(index, 1);
                // If no more listeners for this event, delete the event key
                if (this.eventListeners[eventName].length === 0) {
                    delete this.eventListeners[eventName];
                }
            }
        }

        this.eventTarget.removeEventListener(eventName, callback, options);
    }

    /**
     * Removes all event listeners for the given event name.
     * @param {string} eventName
     */
    removeAllEvents(eventName) {
        if (this.eventListeners[eventName]) {
            this.eventListeners[eventName].forEach(({ callback, options }) => {
                this.eventTarget.removeEventListener(eventName, callback, options);
            });
            delete this.eventListeners[eventName];
        }
    }

    /**
     * Dispatches a custom event with the given name and data.
     * @param {string} eventName
     * @param {any} eventData
     */
    dispatchEvent(eventName, eventData) {
        const event = new CustomEvent(eventName, { detail: eventData });
        this.eventTarget.dispatchEvent(event);
    }

    /**
     * Allows manually re-scanning a specific container for Partial elements.
     * Useful when dynamically adding content to the DOM.
     * @param {HTMLElement} container
     */
    refresh(container = document) {
        this.scanForElements(container);
    }

    /**
     * Clean up SSE connections when elements are removed.
     * @param {HTMLElement} element
     */
    cleanupSSEElement(element) {
        if (this.sseConnections.has(element)) {
            const eventSource = this.sseConnections.get(element);
            eventSource.close();
            this.sseConnections.delete(element);
            element.__xSSEInitialized = false;
        }
    }
}
