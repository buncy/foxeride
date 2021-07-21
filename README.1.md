1) First time payment flow executed 

2) 2nd time customer comes to checkout flow 

    ==> we fectch the cards attached to the customer by hitting the /cards
    ==> display the cards on the client side 
    ==> customer chooses the card he wants to be charged on
    ==> send the card selection details to the server(maybe session on off details too)
    ==> server creates a paymentIntent and confirms it to complete the charge 
    ==> server gets the payment succesful info by webhook 
    ==> server notifies the client if payment successful(need clarity on how this step will happend)