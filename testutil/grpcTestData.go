//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package testutil

// test data for spxctl command
type GRpcTestData struct {
	InputBody    interface{} //Optional. Request body for method
	OutputMsg    string      //Optional. Actual output Message
	OutputBody   interface{} //Optional. Actual output body for json object
	ExpectedMsg  string      //Optional. expected message for the command
	ExpectedBody interface{} //Optional. expected body about json object string
}
