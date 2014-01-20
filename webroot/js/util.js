/**
 * @return An object representing the difference set between two objects.
 */
function difference(a, b) {
    var diff = {};
    for (var key in a) {
        if (key.substring(0, 1) != '$') {
            if (a[key] !== null && typeof a[key] === 'object') {
                diff[key] = difference(a[key], b[key]);
            } else if (a[key] !== b[key]) {
                //console.log('found difference: ' + a[key] + ' != ' + b[key]);
                diff[key] = b[key];
            }
        }
    }
    // Clean out any empty items.
    for (var key in diff) {
        if (typeof diff[key] === 'object' && diff[key] !== null && Object.keys(diff[key]).length === 0) {
            delete diff[key];
        }
    }
    return diff;
}

/**
 * @return value of URL parameter or empty string if no value found.
 */
function getUrlParameter(name) {
    name = name.replace(/[\[]/, '\\\[').replace(/[\]]/, '\\\]');
    var reStr = '[\\?&]' + name + '=([^&#]*)';
    var re = new RegExp(reStr);
    var results = re.exec(window.location.href);
    return results == null ? '' : results[1];
}

/**
 * @return the value of the named cookie.
 */
function getCookie(name) {
    var parts = document.cookie.split(name + '=');
    return parts.length == 2 ? parts.pop().split(';').shift() : null;
}
