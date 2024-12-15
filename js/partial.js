// @ts-check

/**
 * @typedef {Object} PartialOptions
 * @property {'outerHTML'|'innerHTML'} [defaultSwapOption='innerHTML'] - Default swap method.
 * @property {Function} [onError] - Callback function for handling errors.
 * @property {Function|string} [csrfToken] - CSRF token value or function returning the token.
 * @property {boolean} [autoFocus=false] - Whether to auto-focus the target element after content update.
 * @property {number} [debounceTime=0] - Debounce time in milliseconds for event handlers.
 * @property {Function} [onBeforeSwap] - Callback function before swapping content.
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
 * @typedef {Object} RequestParams
 * @property {string} method - The HTTP method.
 * @property {string} url - The request URL.
 * @property {Object} headers - Request headers.
 * @property {string} targetSelector - The CSS selector for the target element.
 * @property {string} partialId - The ID of the target element.
 * @property {Object} paramsObject - Additional parameters.
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
        this.defaultSwapOption = options.defaultSwapOption || 'innerHTML';
        this.autoFocus         = options.autoFocus !== undefined ? options.autoFocus : false;
        this.debounceTime      = options.debounceTime || 0;

        this.eventTarget    = new EventTarget();
        this.eventListeners = {};

        // Store hooks in a single object
        this.hooks = {
            onAction      : [],
            beforeRequest : [],
            afterResponse : [],
            afterSettle   : [],
            beforeSettle  : [],
        };

        this.middleware = [];
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
        this.addHook                    = this.addHook.bind(this);
        this.runHooks                   = this.runHooks.bind(this);
        this.use                        = this.use.bind(this);
        this.runMiddleware              = this.runMiddleware.bind(this);
        this.prepareRequestBody         = this.prepareRequestBody.bind(this);
        this.prepareRequestUrl          = this.prepareRequestUrl.bind(this);
        this.prepareRequestHeaders      = this.prepareRequestHeaders.bind(this);
        this.performRequest             = this.performRequest.bind(this);
        this.performRequestCore         = this.performRequestCore.bind(this);
        this.runMiddleware              = this.runMiddleware.bind(this);

        // Initialize the handler on DOMContentLoaded
        document.addEventListener('DOMContentLoaded', () => this.scanForElements());

        // Listen for popstate events
        window.addEventListener('popstate', this.handlePopState);
    }


    /**
     * Runs all hooks of the specified type, passing along a context object.
     * @param {string} hookName - Name of the hook to run.
     * @param {Object} context - The context object to pass to each callback.
     */
    async runHooks(hookName, context = {}) {
        const callbacks = this.hooks[hookName] || [];
        for (const callback of callbacks) {
            try {
                await callback(context);
            } catch (error) {
                this.handleError(error, context.element, context.targetElement);
            }
        }
    }

    /**
     * Adds a hook callback for the specified hook type.
     * @param {string} hookName - Name of the hook (e.g., 'beforeRequest', 'afterResponse', 'afterSettle').
     * @param {Function} callback - The callback function to register.
     */
    addHook(hookName, callback) {
        if (!this.hooks[hookName]) {
            this.hooks[hookName] = []; // Initialize if not present, to allow custom hooks
        }

        if (typeof callback === 'function') {
            this.hooks[hookName].push(callback);
        } else {
            console.error(`addHook expects a function for hook '${hookName}'`);
        }
    }

    /**
     * Adds middleware to the chain.
     * @param {Function} middleware
     */
    use(middleware) {
        if (typeof middleware === 'function') {
            this.middleware.push(middleware);
        } else {
            console.error('Middleware must be a function.');
        }
    }

    /**
     * Runs middleware in sequence.
     * @param {Object} requestParams
     * @param {Function} finalHandler
     */
    async runMiddleware(requestParams, finalHandler) {
        const stack = [...this.middleware];

        const runner = async () => {
            if (stack.length === 0) {
                // No more middleware, call the final handler and return its result
                return finalHandler();
            }

            const currentMiddleware = stack.shift();
            // Assume middleware is defined as `async (requestParams, next) => { ... }`
            // Middleware can optionally return a value, but commonly it just calls `await next()`.
            return currentMiddleware(requestParams, runner);
        };

        // runner() will either return the finalHandler's result or something middleware returns
        return runner();
    }

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
        const maxRetries = parseInt(retryValue, 10) || 1;

        const requestParams = this.prepareRequestParams(element, { maxRetries: maxRetries });

        const targetElement = document.querySelector(requestParams.targetSelector);
        if (!targetElement) {
            const error = new Error(`No element found with selector '${requestParams.targetSelector}' for 'x-target' targeting.`);
            this.handleError(error, element, targetElement);
            return;
        }

        // Run all onAction hooks
        await this.runHooks('onAction', { element, targetElement, partial: this });

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

            // Dispatch beforeSend event
            this.dispatchEvent('beforeSend', { ...requestParams, element });

            // Call performRequest with the correct parameters
            const responseText = await this.performRequest({
                ...requestParams,
                timeout,
                maxRetries,
            });

            // Dispatch afterReceive event
            this.dispatchEvent('afterReceive', { response: this.lastResponse, element });

            // Process and update the DOM with the response
            await this.processResponse(responseText, targetElement, element);

            // After successfully updating content
            const swapOption = this.getAttributeWithInheritance(element, this.ATTRIBUTES.SWAP) || this.defaultSwapOption;

            if (shouldPushState) {
                const newUrl = new URL(requestParams.url, window.location.origin);
                history.pushState({
                    xPartial: true,
                    partialId: requestParams.partialId,
                    url: newUrl.href,
                    swapOption: swapOption,
                    maxRetries: maxRetries,
                }, '', newUrl);
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
     * @returns {RequestParams} Request parameters
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
     * @returns {RequestParams} Parameters including method, url, headers, body, etc.
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
     * Prepares the request body based on the element's attributes.
     * @param element
     * @param serializeType
     * @param paramsObject
     * @returns {FormData|null|string}
     */
    prepareRequestBody(element, serializeType, paramsObject) {
        const method = element ? this.getMethod(element) : 'GET';
        const isWriteMethod = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method);

        // Check for x-json attribute
        const xJson = element && element.getAttribute(this.ATTRIBUTES.JSON);
        if (xJson) {
            // If x-json is provided, we use it directly
            let bodyData;
            try {
                bodyData = JSON.parse(xJson);
            } catch (e) {
                console.error('Invalid JSON in x-json attribute:', e);
                throw new Error('Invalid JSON in x-json attribute');
            }

            if (paramsObject && Object.keys(paramsObject).length > 0) {
                bodyData = { ...bodyData, ...paramsObject };
            }
            return JSON.stringify(bodyData);
        }

        // If no x-json provided, we rely on form data or nothing
        const form = element && (element.tagName === 'FORM' ? element : element.closest('form'));

        if (serializeType === 'json' || serializeType === 'nested-json' || serializeType === 'xml') {
            if (!form || !(form instanceof HTMLFormElement)) {
                // If we need to serialize form data but no form is found:
                if (isWriteMethod) {
                    throw new Error(`Expected an HTMLFormElement for body serialization, but none found.`);
                } else {
                    // For GET or similar we do not need a body
                    return null;
                }
            }

            let body;
            if (serializeType === 'json') {
                body = Serializer.serializeFormToJson(form);
            } else if (serializeType === 'nested-json') {
                body = Serializer.serializeFormToNestedJson(form);
            } else if (serializeType === 'xml') {
                body = Serializer.serializeFormToXml(form);
            }

            if (paramsObject && Object.keys(paramsObject).length > 0) {
                body = JSON.stringify({ ...JSON.parse(body), ...paramsObject });
            }
            return body;
        } else {
            // Default: FormData
            if (!form || !(form instanceof HTMLFormElement)) {
                if (isWriteMethod) {
                    throw new Error(`Expected an HTMLFormElement for FormData, but none found.`);
                } else {
                    return null;
                }
            }

            const bodyData = new FormData(form);
            if (paramsObject && Object.keys(paramsObject).length > 0) {
                for (const key in paramsObject) {
                    bodyData.append(key, paramsObject[key]);
                }
            }
            return bodyData;
        }
    }

    /**
     * Prepares the request URL with query parameters.
     * @param url string
     * @param paramsObject object
     * @returns {*|string}
     */
    prepareRequestUrl(url, paramsObject) {
        if (!paramsObject || Object.keys(paramsObject).length === 0) {
            return url;
        }

        const queryString = new URLSearchParams(paramsObject).toString();
        return url.includes('?') ? `${url}&${queryString}` : `${url}?${queryString}`;
    }

    /**
     * Prepares the request headers with CSRF token and additional headers.
     * @param element
     * @param defaultHeaders
     * @returns {{}}
     */
    prepareRequestHeaders(element, defaultHeaders = {}) {
        const headers = { ...defaultHeaders };

        if (this.csrfToken) {
            headers['X-CSRF-Token'] =
                typeof this.csrfToken === 'function' ? this.csrfToken() : this.csrfToken;
        }

        if (element) {
            const additionalHeaders = element.getAttribute('x-headers');
            if (additionalHeaders) {
                Object.assign(headers, JSON.parse(additionalHeaders));
            }
        }

        return headers;
    }

    /**
     * Performs the core request using Fetch API with timeout and retries.
     * @param {RequestParams} requestParams
     * @returns {Promise<Response>}
     */
    async performRequestCore(requestParams) {
        const { method, url, headers, body, timeout, maxRetries } = requestParams;

        const controller = new AbortController();
        const options = { method, headers, body, signal: controller.signal };

        let attempts = 0;
        const maxAttempts = maxRetries + 1;

        while (attempts < maxAttempts) {
            attempts++;
            try {
                const timeoutId = timeout
                    ? setTimeout(() => controller.abort(), timeout)
                    : null;

                const response = await fetch(url, options);

                if (timeoutId) clearTimeout(timeoutId);

                if (!response.ok) {
                    // Fetch succeeded but HTTP error
                    const text = await response.text();
                    throw new Error(`HTTP error ${response.status}: ${text}`);
                }

                // Successfully got a response
                return response;

            } catch (error) {
                if (error.name === 'AbortError') {
                    throw new Error('Request timed out');
                }

                // If this was the last attempt, rethrow
                if (attempts >= maxAttempts) {
                    throw error;
                }
                // Otherwise, it will loop and try again
            }
        }

        // If we exit the loop here without returning or throwing, this is a logic flaw.
        // Always throw an error or return something if we don't get a response.
        throw new Error('No response returned from performRequestCore (all attempts failed without producing a response).');
    }

    /**
     * Performs the request and returns the response text.
     * @param {RequestParams} requestParams
     * @returns {Promise<*>}
     */
    async performRequest(requestParams) {
        // Run beforeRequest hooks
        await this.runHooks('beforeRequest', { requestParams, partial: this });

        return await this.runMiddleware(requestParams, async () => {
            const { element, paramsObject } = requestParams;
            const serializeType = element?.getAttribute(this.ATTRIBUTES.SERIALIZE) || 'json';

            const body = this.prepareRequestBody(element, serializeType, paramsObject);
            const url = this.prepareRequestUrl(requestParams.url, paramsObject);
            const headers = this.prepareRequestHeaders(element, requestParams.headers);

            let response;
            try {
                response = await this.performRequestCore({ ...requestParams, url, body, headers });
            } catch (error) {
                console.error('performRequestCore failed:', error);
                throw error;
            }

            if (!response) {
                throw new Error('No response returned from performRequestCore');
            }

            this.lastResponse = response;

            // Run afterResponse hooks
            await this.runHooks('afterResponse', { requestParams, response, partial: this });

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

        // Handle backend instructions
        const backendTargetSelector = this.lastResponse.headers.get('X-Target');
        const backendSwapOption = this.lastResponse.headers.get('X-Swap');
        const infiniteScrollAction = this.lastResponse.headers.get('X-Infinite-Scroll');

        // Determine the target element
        let finalTargetElement = targetElement;
        if (backendTargetSelector) {
            const backendTargetElement = document.querySelector(backendTargetSelector) || document.getElementById(backendTargetSelector);
            if (backendTargetElement) {
                finalTargetElement = backendTargetElement;
            } else if(backendTargetSelector === 'root') {
                // replace the entire body
                finalTargetElement = window;
            }else {
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

        // after all DOM updates
        await this.runHooks('afterSettle', { element, targetElement, partial: this });
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
        // Create a template element to parse the HTML
        const template = document.createElement('template');
        template.innerHTML = newContent.trim();
        const fragment = template.content;

        // Pre-initialize elements within this fragment
        // This runs `setupElement` and attaches event listeners off-DOM
        this.scanForElements(fragment);

        switch (swapOption) {
            case 'outerHTML':
                // Replace the entire target element with the content of the fragment
                targetElement.replaceWith(fragment);
                break;

            case 'innerHTML':
                // Clear and append fragment's children inside targetElement
                targetElement.innerHTML = '';
                targetElement.appendChild(fragment);
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
            const { partialId, url, swapOption, maxRetries } = event.state;
            if (!partialId || !url ) {
                console.warn('No partialId or url found in history.state, falling back to full reload.');
                window.location.reload();
                return;
            }

            const targetElement = document.getElementById(partialId);
            if (!targetElement) {
                console.warn(`No element found with ID '${partialId}'. Falling back to full reload.`);
                window.location.reload();
                return;
            }


            // Re-extract requestParams from the targetElement
            const requestParams = this.extractRequestParams(targetElement);

            requestParams.element = targetElement;
            requestParams.url = url; // Ensure url is set to the popped state URL
            requestParams.maxRetries = maxRetries; // Set a default retry limit
            requestParams.headers["X-Target"] = partialId;

            // Run the request again to restore the state
            try {
                const responseText = await this.performRequest(requestParams);
                const targetElement = document.querySelector(requestParams.targetSelector);
                if (!targetElement) {
                    console.error(`No element found with selector '${requestParams.targetSelector}' for infinite scroll.`);
                    return;
                }

                if (swapOption) {
                    targetElement.setAttribute('x-swap', swapOption);
                }

                await this.processResponse(responseText, targetElement, targetElement);
                if (this.autoFocus) {
                    targetElement.focus();
                }
            } catch (error) {
                this.handleError(error, targetElement);
            }
        } else {
            // No xPartial state, fallback to full page reload or handle differently
            window.location.reload();
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
    static serializeFormToNestedJson(form) {
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