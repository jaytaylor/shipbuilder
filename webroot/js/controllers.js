/**
 * Automatically inject standard paging system to the specified scope.
 *
 * @param scope $scope to use.
 * @param routeParams $routeParams to get `offset` and `limit` from.
 * @param location $location to use to infer base path.
 * @param metaObjectKey string representing the key of the object in `scope` containing the relevant `meta` attribute.
 * @param updateFn Updater function to invoke when paging is triggered.
 *
 * @return wrapped updateFn which also updates the offset & limit params in the location bar.
 */
function injectPaging(scope, routeParams, location, metaObjectKey, updateFn) {
    scope.offset = routeParams.offset || 0;
    scope.limit = '' + (routeParams.limit || 50);

    scope.limits = ['5', '10', '25', '50', '100'];

    var wrappedUpdateFn = function() {
        // Execute updater function.
        updateFn();

        // Generate updated path.
        var path = location.path().replace(/\/\d+\/\d+/, '');
        angular.forEach(['offset', 'limit'], function(key) {
            path += '/' + scope[key];
        });

        // Set new path in location bar.
        location.path(path);
    };

    /**
     * @param url string to extract ``offset`` and ``limit`` values from.
     */
    var urlPaging = function(url) {
        _.map(['offset', 'limit'], function(key) {
            scope[key] = new RegExp(key + '=(\\d+)').exec(url).pop();
        });
        wrappedUpdateFn();
    };

    scope.nextPage = function() { if (scope[metaObjectKey].Meta.next !== null) { urlPaging(scope[metaObjectKey].Meta.next); } };
    scope.previousPage = function() { if (scope[metaObjectKey].Meta.previous !== null) { return urlPaging(scope[metaObjectKey].Meta.previous); } };

    return wrappedUpdateFn;
}

function AppListController($scope, $routeParams, $location, App, promiseTracker) {
    $scope.apps = App.query();
}

