# Transaction ID library

This library supports transaction id handling. It provides methods for checking an http request for an
'X-Request-Id' header, and if present using the value of this as a transaction id.

If the header isn't present, the library will generate a transaction with a 'tid_' prefix
 and a random 10 character string.

Go provides support for passing around variables associated with a request lifecycle via the [context package](https://godoc.org/golang.org/x/net/context)

Best practice for using this is to specify a context as the first argument to your function
 (see [here](https://blog.golang.org/context) for more information along with examples that this library draws on).

## Examples
To extract a transactionID from a request, and create one if none found:

    transactionID := transactionidutils.GetTransactionIDFromRequest(req)

To store that on a context:

    transactionAwareContext := transactionidutils.TransactionAwareContext(context.Background(), transactionID)

NB: context.Background() is a non-nil, empty Context, typically used as the top-level context for incoming requests.

For extracting from a context:

  transactionID, err := GetTransactionIDFromContext(ctx)

It's expected that the transaction ID will be used to output logs. An example using logrus:

	log := log.WithFields(log.Fields{
		transactionIdKey: ctx.Value(transactionIdKey),
	})

NB: this will use the standard transaction id key("transaction_id") that is already used for Content programme log files.
