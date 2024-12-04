// @ts-check

/**
 * @typedef {Object} PartialOptions
 * @property {'outerHTML'|'innerHTML'} [defaultSwapOption='outerHTML'] - Default swap method.
 * @property {Function} [onError] - Callback function for handling errors.
 * @property {Function|string} [csrfToken] - CSRF token value or function returning the token.
 * @property {Function} [beforeRequest] - Hook before the request is sent.
 * @property {Function} [afterResponse] - Hook after the response is received.
 * @property {boolean} [autoFocus=false] - Whether to auto-focus the target element after content update.
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
                PATCH:  'x-patch',
            },
            TARGET:          'x-target',
            TRIGGER:         'x-trigger',
            SERIALIZE:       'x-serialize',
            JSON:            'x-json',
            PARAMS:          'x-params',
            SWAP_OOB:        'x-swap-oob',
            PUSH_STATE:      'x-push-state',
            FOCUS:           'x-focus',
            DEBOUNCE:        'x-debounce',
            BEFORE:          'x-before',
            AFTER:           'x-after',
            SSE:             'x-sse',
            INDICATOR:       'x-indicator',
            CONFIRM:         'x-confirm',
            TIMEOUT:         'x-timeout',
            RETRY:           'x-retry',
            ON_ERROR:        'x-on-error',
            LOADING_CLASS:   'x-loading-class',
            SWAP:            'x-swap',
            INFINITE_SCROLL: 'x-infinite-scroll',
        };

        this.SERIALIZE_TYPES = {
            JSON:        'json',
            NESTED_JSON: 'nested-json',
            XML:         'xml',
        };

        this.INHERITABLE_ATTRIBUTES = [
            this.ATTRIBUTES.TARGET,
            this.ATTRIBUTES.SWAP,
            this.ATTRIBUTES.SERIALIZE,
            this.ATTRIBUTES.TRIGGER,
            this.ATTRIBUTES.LOADING_CLASS,
            this.ATTRIBUTES.INDICATOR,
            this.ATTRIBUTES.RETRY,
            this.ATTRIBUTES.TIMEOUT,
            this.ATTRIBUTES.FOCUS,
            this.ATTRIBUTES.DEBOUNCE,
        ];

        // Store options with default values
        this.onError           = options.onError || null;
        this.csrfToken         = options.csrfToken || null;
        this.defaultSwapOption = options.defaultSwapOption || 'outerHTML';
        this.beforeRequest     = options.beforeRequest || null;
        this.afterResponse     = options.afterResponse || null;
        this.autoFocus         = options.autoFocus !== undefined ? options.autoFocus : false;
        this.debounceTime      = options.debounceTime || 0;

        this.eventTarget    = new EventTarget();
        this.eventListeners = {};

        // Map to store SSE connections per element
        this.sseConnections = new Map();

        // Bind methods to ensure correct 'this' context
        this.scanForElements            = this.scanForElements.bind(this);
        this.setupElement               = this.setupElement.bind(this);
        this.setupSSEElement            = this.setupSSEElement.bind(this);
        this.setupInfiniteScroll        = this.setupInfiniteScroll.bind(this);
        this.stopInfiniteScroll         = this.stopInfiniteScroll.bind(this);
        this.handleAction               = this.handleAction.bind(this);
        this.handleOobSwapping          = this.handleOobSwapping.bind(this);
        this.handlePopState             = this.handlePopState.bind(this);
        this.handleInfiniteScrollAction = this.handleInfiniteScrollAction.bind(this);

        // Initialize the handler on DOMContentLoaded
        document.addEventListener('DOMContentLoaded', () => this.scanForElements());

        // Listen for popstate events
        window.addEventListener('popstate', this.handlePopState);
    }

    // Initialization Methods
    // ----------------------

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

    // SSE Methods
    // -----------

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
            this.handleSSEMessage(event, element).catch(error => {
                this.handleError(error, element);
            });
        };

        eventSource.onerror = (error) => {
            this.handleError(error, element);
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

            this.performSwap(targetElement, data.content, swapOption);

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
            this.handleError(error, element);
        }
    }

    // Element Setup Methods
    // ---------------------

    /**
     * Sets up an individual element by attaching the appropriate event listener.
     * @param {HTMLElement} element
     */
    setupElement(element) {
        // Avoid attaching multiple listeners
        if (element.__xRequestHandlerInitialized) return;

        // Check for x-infinite-scroll attribute
        if (element.hasAttribute(this.ATTRIBUTES.INFINITE_SCROLL)) {
            this.setupInfiniteScroll(element);
            // Mark the element as initialized
            element.__xRequestHandlerInitialized = true;
            return;
        }

        // Set a default trigger based on the element type
        let trigger;
        if (element.tagName === 'FORM') {
            trigger = element.getAttribute(this.ATTRIBUTES.TRIGGER) || 'submit';
        } else {
            trigger = this.getAttributeWithInheritance(element, this.ATTRIBUTES.TRIGGER) || 'click';
        }

        // Get custom debounce time from x-debounce attribute
        let elementDebounceTime = this.debounceTime; // Default to global debounce time
        const xDebounce = this.getAttributeWithInheritance(element, this.ATTRIBUTES.DEBOUNCE);
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
            this.handleAction(event, element).catch(error => {
                this.handleError(error, element);
            });
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

    // Infinite Scroll Methods
    // -----------------------

    /**
     * Sets up infinite scroll on an element.
     * @param {HTMLElement} parentElement
     */
    setupInfiniteScroll(parentElement) {
        // Check if infinite scroll has been stopped
        if (parentElement.__infiniteScrollStopped) {
            return;
        }

        // Create or find the sentinel element
        let sentinel = parentElement.__sentinelElement;
        if (!sentinel) {
            sentinel = document.createElement('div');
            sentinel.classList.add('infinite-scroll-sentinel');
            parentElement.parentNode.insertBefore(sentinel, parentElement.nextSibling);
            parentElement.__sentinelElement = sentinel;
        }

        // Set up Intersection Observer on the sentinel
        const observer = new IntersectionObserver((entries) => {
            entries.forEach(entry => {
                if (entry.isIntersecting) {
                    // Unobserve to prevent multiple triggers
                    observer.unobserve(sentinel);
                    // Execute the action
                    this.handleInfiniteScrollAction(parentElement).catch(error => {
                        this.handleError(error, parentElement);
                    });
                }
            });
        });

        observer.observe(sentinel);

        // Store the observer reference
        parentElement.__infiniteScrollObserver = observer;
    }

    /**
     * Stops the infinite scroll by removing the sentinel and disconnecting the observer.
     * @param {HTMLElement} parentElement
     */
    stopInfiniteScroll(parentElement) {
        // Remove the sentinel element
        if (parentElement.__sentinelElement) {
            parentElement.__sentinelElement.remove();
            delete parentElement.__sentinelElement;
        }

        // Set a flag to indicate infinite scroll has stopped
        parentElement.__infiniteScrollStopped = true;

        // Disconnect the observer
        if (parentElement.__infiniteScrollObserver) {
            parentElement.__infiniteScrollObserver.disconnect();
            delete parentElement.__infiniteScrollObserver;
        }
    }

    /**
     * Handles the action for infinite scroll.
     * @param {HTMLElement} parentElement
     */
    async handleInfiniteScrollAction(parentElement) {
        const url = parentElement.getAttribute(this.ATTRIBUTES.ACTIONS.GET);
        if (!url) {
            console.error('No URL specified for infinite scroll.');
            return;
        }

        const requestParams = this.prepareRequestParams(parentElement, { maxRetries: 2 });

        // Set X-Action header if not already set
        if (!requestParams.headers["X-Action"]) {
            requestParams.headers["X-Action"] = "infinite-scroll";
        }

        // Get the params from the last child
        requestParams.paramsObject = this.getChildParamsObject(parentElement);
        if (requestParams.paramsObject && Object.keys(requestParams.paramsObject).length > 0) {
            requestParams.headers["X-Params"] = JSON.stringify(requestParams.paramsObject);
        }

        try {
            const responseText = await this.performRequest(requestParams);
            const targetElement = document.querySelector(requestParams.targetSelector);
            if (!targetElement) {
                console.error(`No element found with selector '${requestParams.targetSelector}' for infinite scroll.`);
                return;
            }

            await this.processResponse(responseText, targetElement, parentElement);

            // Re-attach the observer to continue loading
            this.setupInfiniteScroll(parentElement);
        } catch (error) {
            this.handleError(error, parentElement, parentElement);
        }
    }

    /**
     * Retrieves parameters from the last child element.
     * @param {HTMLElement} parentElement
     * @returns {Object}
     */
    getChildParamsObject(parentElement) {
        // Get x-params from the last child
        const lastChild = parentElement.lastElementChild;
        let paramsObject = {};
        if (lastChild) {
            const xParamsAttr = lastChild.getAttribute(this.ATTRIBUTES.PARAMS);
            if (xParamsAttr) {
                try {
                    paramsObject = JSON.parse(xParamsAttr);
                } catch (e) {
                    console.error('Invalid JSON in x-params attribute of last child:', e);
                }
            }
        }

        return paramsObject;
    }

    // Action Handling Methods
    // -----------------------

    /**
     * Handles the action when an element is triggered.
     * @param {Event} event
     * @param {HTMLElement} element
     */
    async handleAction(event, element) {
        // Get a confirmation message from x-confirm
        const confirmMessage = element.getAttribute(this.ATTRIBUTES.CONFIRM);
        if (confirmMessage) {
            const confirmed = window.confirm(confirmMessage);
            if (!confirmed) {
                return; // Abort the action
            }
        }

        // Get the indicator selector from x-indicator
        const indicatorSelector = this.getAttributeWithInheritance(element, this.ATTRIBUTES.INDICATOR);
        let indicatorElement = null;
        if (indicatorSelector) {
            indicatorElement = document.querySelector(indicatorSelector);
        }

        // Get loading class from x-loading-class
        const loadingClass = this.getAttributeWithInheritance(element, this.ATTRIBUTES.LOADING_CLASS);

        // Handle x-focus
        const focusEnabled = this.getAttributeWithInheritance(element, this.ATTRIBUTES.FOCUS) !== 'false';

        // Handle x-push-state
        const shouldPushState = this.getAttributeWithInheritance(element, this.ATTRIBUTES.PUSH_STATE) !== 'false';

        // Handle x-timeout
        const timeoutValue = this.getAttributeWithInheritance(element, this.ATTRIBUTES.TIMEOUT);
        const timeout = parseInt(timeoutValue, 10);

        // Handle x-retry
        const retryValue = this.getAttributeWithInheritance(element, this.ATTRIBUTES.RETRY);
        const maxRetries = parseInt(retryValue, 10) || 0;

        const requestParams = this.prepareRequestParams(element);

        const targetElement = document.querySelector(requestParams.targetSelector);
        if (!targetElement) {
            const error = new Error(`No element found with selector '${requestParams.targetSelector}' for 'x-target' targeting.`);
            this.handleError(error, element, targetElement);
            return;
        }

        try {
            // Show the indicator before the request
            if (indicatorElement) {
                indicatorElement.style.display = ''; // Or apply a CSS class to show
            }

            // Add loading class to target element
            if (loadingClass && targetElement) {
                targetElement.classList.add(loadingClass);
            }

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

            // Call performRequest with the correct parameters
            const responseText = await this.performRequest({
                ...requestParams,
                timeout,
                maxRetries,
            });

            // After response hook
            if (typeof this.afterResponse === 'function') {
                await this.afterResponse({ response: this.lastResponse, element });
            }

            // Dispatch afterReceive event
            this.dispatchEvent('afterReceive', { response: this.lastResponse, element });

            // Process and update the DOM with the response
            await this.processResponse(responseText, targetElement, element);

            // After successfully updating content
            if (shouldPushState) {
                const newUrl = new URL(requestParams.url, window.location.origin);
                history.pushState({ xPartial: true }, '', newUrl);
            }

            // Dispatch x-after event(s) if specified
            const afterEvents = element.getAttribute(this.ATTRIBUTES.AFTER);
            if (afterEvents) {
                await this.dispatchCustomEvents(afterEvents, { element, event });
            }

            // Auto-focus if enabled
            if (this.autoFocus && focusEnabled) {
                if (targetElement.getAttribute('tabindex') === null) {
                    targetElement.setAttribute('tabindex', '-1');
                }
                targetElement.focus();
            }

        } catch (error) {
            const onErrorAttr = element.getAttribute(this.ATTRIBUTES.ON_ERROR);
            if (onErrorAttr && typeof window[onErrorAttr] === 'function') {
                window[onErrorAttr](error, element);
            } else if (typeof this.onError === 'function') {
                this.onError(error, element);
            } else {
                // Default error handling
                console.error('Request failed:', error);
                targetElement.innerHTML = `<div class="error">An error occurred: ${error.message}</div>`;
            }
        } finally {
            // Hide the indicator after the request completes or fails
            if (indicatorElement) {
                indicatorElement.style.display = 'none'; // Or remove the CSS class
            }

            // Remove loading class from target element
            if (loadingClass && targetElement) {
                targetElement.classList.remove(loadingClass);
            }
        }
    }

    // Request Preparation Methods
    // ---------------------------

    /**
     * Prepares the request parameters for the Fetch API.
     * @param {HTMLElement} element
     * @param {Object} [additionalParams={}]
     * @returns {Object} Request parameters
     */
    prepareRequestParams(element, additionalParams = {}) {
        const requestParams = this.extractRequestParams(element);
        requestParams.element = element;

        if (!requestParams.url) {
            throw new Error(`No URL specified for method ${requestParams.method} on element.`);
        }

        const targetElement = document.querySelector(requestParams.targetSelector);
        if (!targetElement) {
            throw new Error(`No element found with selector '${requestParams.targetSelector}' for 'x-target' targeting.`);
        }

        if (!requestParams.partialId) {
            throw new Error(`Target element does not have an 'id' attribute.`);
        }

        // Set the X-Target header
        requestParams.headers["X-Target"] = requestParams.partialId;

        // Merge additional parameters
        Object.assign(requestParams, additionalParams);

        return requestParams;
    }

    /**
     * Extracts request parameters from the element.
     * @param {HTMLElement} element
     * @returns {Object} Parameters including method, url, headers, body, etc.
     */
    extractRequestParams(element) {
        const method = this.getMethod(element);
        const actionAttr = `x-${method.toLowerCase()}`;
        let url = this.getAttributeWithInheritance(element, actionAttr);

        const headers = this.getHeaders(element);

        let targetSelector = this.getAttributeWithInheritance(element, this.ATTRIBUTES.TARGET);
        if (!targetSelector) {
            targetSelector = element.id ? `#${element.id}` : "body";
        }

        const targetElement = document.querySelector(targetSelector);
        const partialId = targetElement ? targetElement.getAttribute('id') : null;

        const xParams = this.getAttributeWithInheritance(element, this.ATTRIBUTES.PARAMS);
        let paramsObject = {};

        if (xParams) {
            try {
                paramsObject = JSON.parse(xParams);
            } catch (e) {
                const error = new Error('Invalid JSON in x-params attribute');
                this.handleError(error, element, targetElement);
            }
        }

        return { method, url, headers, targetSelector, partialId, paramsObject };
    }

    /**
     * Determines the HTTP method based on the element's attributes.
     * @param {HTMLElement} element
     * @returns {string} HTTP method
     */
    getMethod(element) {
        for (const attr of Object.values(this.ATTRIBUTES.ACTIONS)) {
            if (this.hasAttributeWithInheritance(element, attr)) {
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

        // List of attributes to exclude from headers
        const excludedAttributes = [
            ...Object.values(this.ATTRIBUTES.ACTIONS),
            this.ATTRIBUTES.TARGET,
            this.ATTRIBUTES.TRIGGER,
            this.ATTRIBUTES.SWAP,
            this.ATTRIBUTES.SWAP_OOB,
            this.ATTRIBUTES.PUSH_STATE,
            this.ATTRIBUTES.INFINITE_SCROLL,
            this.ATTRIBUTES.DEBOUNCE
        ];

        // Collect x-* attributes to include as headers
        for (const attr of element.attributes) {
            const name = attr.name;
            if (name.startsWith('x-') && !excludedAttributes.includes(name)) {
                const headerName = 'X-' + this.capitalize(name.substring(2)); // Remove 'x-' prefix and capitalize
                headers[headerName] = attr.value;
            }
        }

        return headers;
    }

    // Utility Methods
    // ---------------

    /**
     * Retrieves the value of an attribute from the element or its ancestors.
     * @param {HTMLElement} element
     * @param {string} attributeName
     * @returns {string|null}
     */
    getAttributeWithInheritance(element, attributeName) {
        if (!this.INHERITABLE_ATTRIBUTES.includes(attributeName)) {
            return element.getAttribute(attributeName);
        }

        let currentElement = element;
        while (currentElement) {
            if (currentElement.hasAttribute(attributeName)) {
                return currentElement.getAttribute(attributeName);
            }
            currentElement = currentElement.parentElement;
        }
        return null;
    }

    /**
     * Checks if an attribute exists on the element or its ancestors.
     * @param {HTMLElement} element
     * @param {string} attributeName
     * @returns {boolean}
     */
    hasAttributeWithInheritance(element, attributeName) {
        return this.getAttributeWithInheritance(element, attributeName) !== null;
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

    // Request Execution Methods
    // -------------------------

    /**
     * Performs the HTTP request using Fetch API.
     * @param {Object} requestParams - Parameters including method, url, headers, body, etc.
     * @returns {Promise<string>} Response text
     */
    async performRequest(requestParams) {
        const { method, url, headers, element, timeout, maxRetries, paramsObject } = requestParams;
        let requestUrl = url;

        const controller = new AbortController();
        const options = {
            method,
            headers,
            credentials: 'same-origin',
            signal: controller.signal,
        };

        // Handle x-serialize attribute
        const serializeType = element && (
            element.getAttribute(this.ATTRIBUTES.SERIALIZE) ||
            (element.closest('form') && element.closest('form').getAttribute(this.ATTRIBUTES.SERIALIZE))
        );

        // Check for x-json attribute
        const xJson = element && element.getAttribute(this.ATTRIBUTES.JSON);

        // Handle request body
        if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
            let bodyData = {};

            if (xJson) {
                // Parse x-json attribute
                try {
                    bodyData = JSON.parse(xJson);
                } catch (e) {
                    console.error('Invalid JSON in x-json attribute:', e);
                    throw new Error('Invalid JSON in x-json attribute');
                }
            } else if (element && (element.tagName === 'FORM' || element.closest('form'))) {
                const form = element.tagName === 'FORM' ? element : element.closest('form');
                if (serializeType === this.SERIALIZE_TYPES.JSON) {
                    // Serialize form data as flat JSON
                    bodyData = JSON.parse(Serializer.serializeFormToJson(form));
                } else if (serializeType === this.SERIALIZE_TYPES.NESTED_JSON) {
                    // Serialize form data as nested JSON
                    bodyData = JSON.parse(Serializer.serializeFormToNestedJson(form));
                } else if (serializeType === this.SERIALIZE_TYPES.XML) {
                    // Serialize form data as XML
                    bodyData = Serializer.serializeFormToXml(form);
                    headers['Content-Type'] = 'application/xml';
                } else {
                    // Use FormData
                    bodyData = new FormData(form);
                }
            }

            // Merge paramsObject with bodyData
            if (paramsObject && Object.keys(paramsObject).length > 0) {
                if (bodyData instanceof FormData) {
                    // Append params to FormData
                    for (const key in paramsObject) {
                        bodyData.append(key, paramsObject[key]);
                    }
                } else if (typeof bodyData === 'string') {
                    // Parse existing bodyData and merge
                    bodyData = { ...JSON.parse(bodyData), ...paramsObject };
                } else {
                    // Merge objects
                    bodyData = { ...bodyData, ...paramsObject };
                }
            }

            if (bodyData instanceof FormData) {
                options.body = bodyData;
            } else if (typeof bodyData === 'string') {
                options.body = bodyData;
                headers['Content-Type'] = headers['Content-Type'] || 'application/json';
            } else {
                options.body = JSON.stringify(bodyData);
                headers['Content-Type'] = headers['Content-Type'] || 'application/json';
            }
        } else {
            // For GET requests, append params to URL
            if (paramsObject && Object.keys(paramsObject).length > 0) {
                const urlParams = new URLSearchParams(paramsObject).toString();
                requestUrl += (requestUrl.includes('?') ? '&' : '?') + urlParams;
            }
        }

        // Start the timeout if specified
        let timeoutId;
        if (!isNaN(timeout) && timeout > 0) {
            timeoutId = setTimeout(() => {
                controller.abort();
            }, timeout);
        }

        let attempts = 0;
        const maxAttempts = maxRetries + 1;

        while (attempts < maxAttempts) {
            attempts++;
            try {
                const response = await fetch(requestUrl, options);
                if (timeoutId) {
                    clearTimeout(timeoutId);
                }

                this.lastResponse = response;

                if (!response.ok) {
                    const text = await response.text();
                    throw new Error(`HTTP error ${response.status}: ${text}`);
                }
                return response.text();
            } catch (error) {
                if (timeoutId) {
                    clearTimeout(timeoutId);
                }

                if (error.name === 'AbortError') {
                    throw new Error('Request timed out');
                }

                if (attempts >= maxAttempts) {
                    throw error;
                }
                // TODO, implement a delay before retrying
            }
        }
    }

    // Response Processing Methods
    // ---------------------------

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

        // Handle backend instructions
        const backendTargetSelector = this.lastResponse.headers.get('X-Target');
        const backendSwapOption = this.lastResponse.headers.get('X-Swap');
        const infiniteScrollAction = this.lastResponse.headers.get('X-Infinite-Scroll');

        // Determine the target element
        let finalTargetElement = targetElement;
        if (backendTargetSelector) {
            const backendTargetElement = document.querySelector(backendTargetSelector);
            if (backendTargetElement) {
                finalTargetElement = backendTargetElement;
            } else {
                console.error(`No element found with selector '${backendTargetSelector}' specified in X-Target header.`);
            }
        }

        // Determine the swap option
        let swapOption = this.getAttributeWithInheritance(element, this.ATTRIBUTES.SWAP) || this.defaultSwapOption;
        if (backendSwapOption) {
            swapOption = backendSwapOption;
        }

        // Get the content from the response
        const newContent = doc.body.innerHTML;

        // Replace the target's content
        this.performSwap(finalTargetElement, newContent, swapOption);

        // Dispatch afterUpdate event
        this.dispatchEvent('afterUpdate', { targetElement: finalTargetElement, element });

        // Re-scan the newly added content for Partial elements
        this.scanForElements(finalTargetElement);

        // Handle OOB swapping with the extracted OOB elements
        this.handleOobSwapping(oobElements);

        // Handle any x-event-* headers from the response
        await this.handleResponseEvents();

        // Stop infinite scroll if instructed by backend
        if (infiniteScrollAction === 'stop' && element.hasAttribute(this.ATTRIBUTES.INFINITE_SCROLL)) {
            this.stopInfiniteScroll(element);
        }
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

            const newContent = oobElement.outerHTML;

            this.performSwap(existingElement, newContent, swapOption);

            // After swapping, initialize any new elements within the replaced content
            const newElement = document.getElementById(targetId);
            if (newElement) {
                this.scanForElements(newElement);
            }
        });
    }

    /**
     * Performs the swap operation on the target element based on the swap option.
     * @param {HTMLElement} targetElement
     * @param {string} newContent
     * @param {string} swapOption
     */
    performSwap(targetElement, newContent, swapOption) {
        switch (swapOption) {
            case 'innerHTML':
                targetElement.innerHTML = newContent;
                break;
            case 'outerHTML':
                targetElement.outerHTML = newContent;
                break;
            case 'beforebegin':
            case 'afterbegin':
            case 'beforeend':
            case 'afterend':
                targetElement.insertAdjacentHTML(swapOption, newContent);
                break;
            default:
                console.error(`Invalid swap option '${swapOption}'. Using 'innerHTML' as default.`);
                targetElement.innerHTML = newContent;
                break;
        }
    }

    /**
     * Handles any x-event-* headers from the response and dispatches events accordingly.
     */
    async handleResponseEvents() {
        if (!this.lastResponse || !this.lastResponse.headers) {
            return;
        }

        this.lastResponse.headers.forEach((value, name) => {
            const lowerName = name.toLowerCase();
            if (lowerName.startsWith('x-event-')) {
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

    // Event Handling Methods
    // ----------------------

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
                this.handleError(error, document.body);
            }
        }
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

    // Cleanup Methods
    // ---------------

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

    // Error Handling Methods
    // ----------------------

    /**
     * Handles errors by calling the provided error callback or logging to the console.
     * @param {Error} error
     * @param {HTMLElement} element
     * @param {HTMLElement} [targetElement]
     */
    handleError(error, element, targetElement = null) {
        if (typeof this.onError === 'function') {
            this.onError(error, element);
        } else {
            console.error('Error:', error);
            if (targetElement) {
                targetElement.innerHTML = `<div class="error">An error occurred: ${error.message}</div>`;
            }
        }
    }
}

class Serializer {
    /**
     * Serializes form data to a flat JSON string.
     * @param {HTMLFormElement} form
     * @returns {string} JSON string
     */
    static serializeFormToJson(form) {
        const formData = new FormData(form);
        const jsonObject = {};
        formData.forEach((value, key) => {
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
        return JSON.stringify(jsonObject);
    }

    /**
     * Serializes form data to a nested JSON string.
     * @param {HTMLFormElement} form
     * @returns {string} Nested JSON string
     */
    serializeFormToNestedJson(form) {
        const formData = new FormData(form);
        const serializedData = {};

        for (let [name, value] of formData) {
            const inputElement = form.querySelector(`[name="${name}"]`);
            const checkBoxCustom = form.querySelector(`[data-custom="true"]`);
            const inputType = inputElement ? inputElement.type : null;
            const inputStep = inputElement ? inputElement.step : null;

            // Check if the input type is number and convert the value if so
            if (inputType === 'number') {
                if (inputStep && inputStep !== "any" && Number(inputStep) % 1 === 0) {
                    value = parseInt(value, 10);
                } else if (inputStep === "any") {
                    value = value.includes('.') ? parseFloat(value) : parseInt(value, 10);
                } else {
                    value = parseFloat(value);
                }
            }

            // Check if the input type is checkbox and convert the value to boolean
            if (inputType === 'checkbox' && !checkBoxCustom) {
                value = inputElement.checked; // value will be true if checked, false otherwise
            }

            // Check if the input type is select-one and has data-bool attribute
            if (inputType === 'select-one' && inputElement.getAttribute('data-bool') === 'true') {
                value = value === "true"; // Value will be true if selected, false otherwise
            }

            // Attempt to parse JSON strings
            try {
                value = JSON.parse(value);
            } catch (e) {
                // If parsing fails, treat as a simple string
            }

            const keys = name.split(/[.[\]]+/).filter(Boolean); // split by dot or bracket notation
            let obj = serializedData;

            for (let i = 0; i < keys.length - 1; i++) {
                if (!obj[keys[i]]) {
                    obj[keys[i]] = /^\d+$/.test(keys[i + 1]) ? [] : {}; // create an array if the next key is an index
                }
                obj = obj[keys[i]];
            }

            const lastKey = keys[keys.length - 1];
            if (lastKey in obj && Array.isArray(obj[lastKey])) {
                obj[lastKey].push(value); // add to array if the key already exists
            } else if (lastKey in obj) {
                obj[lastKey] = [obj[lastKey], value];
            } else {
                obj[lastKey] = value; // set value for key
            }
        }

        return JSON.stringify(serializedData);
    }

    /**
     * Serializes form data to an XML string.
     * @param {HTMLFormElement} form
     * @returns {string} XML string
     */
    static serializeFormToXml(form) {
        const formData = new FormData(form);
        let xmlString = '<?xml version="1.0" encoding="UTF-8"?><form>';

        formData.forEach((value, key) => {
            xmlString += `<${key}>${this.escapeXml(value)}</${key}>`;
        });

        xmlString += '</form>';
        return xmlString;
    }

    /**
     * Escapes XML special characters.
     * @param {string} unsafe
     * @returns {string}
     */
    static escapeXml(unsafe) {
        return unsafe.replace(/[<>&'"]/g, function (c) {
            switch (c) {
                case '<': return '&lt;';
                case '>': return '&gt;';
                case '&': return '&amp;';
                case '\'': return '&apos;';
                case '"': return '&quot;';
                default: return c;
            }
        });
    }
}
